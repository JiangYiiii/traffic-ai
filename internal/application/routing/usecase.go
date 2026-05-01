package routing

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/trailyai/traffic-ai/internal/application/oauth"
	domainModel "github.com/trailyai/traffic-ai/internal/domain/model"
	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

// UseCase implements domainRouting.RoutingService and manages tokenGroup CRUD.
type UseCase struct {
	tgRepo      domainRouting.TokenGroupRepository
	modelRepo   domainModel.ModelRepository
	accountRepo domainModel.ModelAccountRepository
	aesKey      []byte
	oauthCfg    config.OAuthConfig
	breaker     domainRouting.CircuitBreaker // nil 表示未启用熔断，选号时不过滤
}

func NewUseCase(
	tgRepo domainRouting.TokenGroupRepository,
	modelRepo domainModel.ModelRepository,
	accountRepo domainModel.ModelAccountRepository,
	aesKey []byte,
	oauthCfg config.OAuthConfig,
	breaker domainRouting.CircuitBreaker,
) *UseCase {
	return &UseCase{
		tgRepo:      tgRepo,
		modelRepo:   modelRepo,
		accountRepo: accountRepo,
		aesKey:      aesKey,
		oauthCfg:    oauthCfg,
		breaker:     breaker,
	}
}

// ---- TokenGroup CRUD ----

func (uc *UseCase) CreateTokenGroup(ctx context.Context, tg *domainRouting.TokenGroup) error {
	existing, err := uc.tgRepo.FindByName(ctx, tg.Name)
	if err != nil {
		logger.L.Errorw("find token group by name failed", "error", err)
		return errcode.ErrInternal
	}
	if existing != nil {
		return errcode.ErrDuplicateTokenGroup
	}
	if err := uc.tgRepo.Create(ctx, tg); err != nil {
		logger.L.Errorw("create token group failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) ListTokenGroups(ctx context.Context) ([]*domainRouting.TokenGroup, error) {
	list, err := uc.tgRepo.List(ctx)
	if err != nil {
		logger.L.Errorw("list token groups failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return list, nil
}

func (uc *UseCase) AddModelAccountToGroup(ctx context.Context, tokenGroupID, modelAccountID int64) error {
	tg, err := uc.tgRepo.FindByID(ctx, tokenGroupID)
	if err != nil {
		logger.L.Errorw("find token group failed", "error", err)
		return errcode.ErrInternal
	}
	if tg == nil {
		return errcode.ErrTokenGroupNotFound
	}
	a, err := uc.accountRepo.FindByID(ctx, modelAccountID)
	if err != nil {
		logger.L.Errorw("find model account failed", "error", err)
		return errcode.ErrInternal
	}
	if a == nil {
		return errcode.ErrModelAccountNotFound
	}
	if err := uc.tgRepo.AddModelAccount(ctx, tokenGroupID, modelAccountID); err != nil {
		logger.L.Errorw("add model account to group failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) RemoveModelAccountFromGroup(ctx context.Context, tokenGroupID, modelAccountID int64) error {
	if err := uc.tgRepo.RemoveModelAccount(ctx, tokenGroupID, modelAccountID); err != nil {
		logger.L.Errorw("remove model account from group failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

// ListModelAccountIDsForGroup returns model account IDs bound to a token group (for admin UI).
func (uc *UseCase) ListModelAccountIDsForGroup(ctx context.Context, tokenGroupID int64) ([]int64, error) {
	tg, err := uc.tgRepo.FindByID(ctx, tokenGroupID)
	if err != nil {
		logger.L.Errorw("find token group failed", "error", err)
		return nil, errcode.ErrInternal
	}
	if tg == nil {
		return nil, errcode.ErrTokenGroupNotFound
	}
	ids, err := uc.tgRepo.ListModelAccountIDs(ctx, tokenGroupID)
	if err != nil {
		logger.L.Errorw("list model account ids for group failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return ids, nil
}

// ---- RoutingService implementation ----

// SelectModelAccount picks one model account by weighted random from candidates
// matching tokenGroup + modelName + protocol.
func (uc *UseCase) SelectModelAccount(ctx context.Context, tokenGroup, modelName, protocol string) (*domainRouting.RouteResult, error) {
	return uc.selectModelAccountCore(ctx, tokenGroup, modelName, protocol, nil)
}

// SelectModelAccountExcluding 同 SelectModelAccount，但会跳过给定的 excludeIDs。
// 卡 #3b 的 fallback 循环会使用本方法：前一个账号失败后排除它重新选号。
func (uc *UseCase) SelectModelAccountExcluding(ctx context.Context, tokenGroup, modelName, protocol string, excludeIDs []int64) (*domainRouting.RouteResult, error) {
	return uc.selectModelAccountCore(ctx, tokenGroup, modelName, protocol, excludeIDs)
}

// SelectOpenAICompatibleAccount 见 domainRouting.RoutingService。
func (uc *UseCase) SelectOpenAICompatibleAccount(ctx context.Context, tokenGroup, modelHint string) (*domainRouting.RouteResult, error) {
	if strings.TrimSpace(modelHint) != "" {
		return uc.SelectModelAccount(ctx, tokenGroup, strings.TrimSpace(modelHint), "openai")
	}

	accountIDs, err := uc.tgRepo.ListModelAccountIDsByName(ctx, tokenGroup)
	if err != nil {
		logger.L.Errorw("routing: list model account ids failed", "error", err, "group", tokenGroup)
		return nil, errcode.ErrInternal
	}
	if len(accountIDs) == 0 {
		return nil, errcode.ErrNoAvailableRoute
	}

	accounts, err := uc.accountRepo.ListByIDs(ctx, accountIDs)
	if err != nil {
		logger.L.Errorw("routing: list model accounts by ids failed", "error", err)
		return nil, errcode.ErrInternal
	}

	var candidates []*domainModel.ModelAccount
	for _, a := range accounts {
		if !a.IsActive {
			continue
		}
		if !routingProtocolMatches("openai", a.Protocol) {
			continue
		}
		if uc.breaker != nil {
			allowed, bErr := uc.breaker.Allow(ctx, a.ID)
			if bErr != nil {
				logger.L.Warnw("routing: circuit breaker Allow failed, allowing conservatively",
					"modelAccountID", a.ID, "error", bErr)
			} else if !allowed {
				continue
			}
		}
		m, ferr := uc.modelRepo.FindByID(ctx, a.ModelID)
		if ferr != nil || m == nil || !m.IsActive || !m.IsListed {
			continue
		}
		candidates = append(candidates, a)
	}

	if len(candidates) == 0 {
		return nil, errcode.ErrNoAvailableRoute
	}

	chosen := weightedRandom(candidates)
	m, err := uc.modelRepo.FindByID(ctx, chosen.ModelID)
	if err != nil || m == nil {
		logger.L.Errorw("routing: find model for chosen account failed", "error", err, "modelAccountID", chosen.ID)
		return nil, errcode.ErrInternal
	}

	if err := uc.materializeAccountCredentials(ctx, chosen, m); err != nil {
		return nil, err
	}
	return &domainRouting.RouteResult{Account: chosen, Model: m}, nil
}

// materializeAccountCredentials 解密 credential，并在 OAuth 将过期时刷新 access_token。
func (uc *UseCase) materializeAccountCredentials(ctx context.Context, chosen *domainModel.ModelAccount, m *domainModel.Model) error {
	plain, err := crypto.DecryptAES(chosen.Credential, uc.aesKey)
	if err != nil {
		logger.L.Errorw("routing: decrypt credential failed", "error", err, "modelAccountID", chosen.ID)
		return errcode.ErrInternal
	}
	chosen.Credential = plain

	if chosen.AuthType == "oauth_authorization_code" && chosen.TokenExpiresAt != nil {
		if time.Until(*chosen.TokenExpiresAt) < 5*time.Minute {
			refreshToken := ""
			if chosen.RefreshToken != "" {
				rt, decErr := crypto.DecryptAES(chosen.RefreshToken, uc.aesKey)
				if decErr == nil {
					refreshToken = rt
				} else {
					logger.L.Warnw("routing: decrypt refresh_token failed", "modelAccountID", chosen.ID)
				}
			}
			if refreshToken != "" {
				providerID := ""
				if m != nil {
					providerID = m.Provider
				}
				if providerID != "" {
					newAccess, newRefresh, expiresIn, refreshErr := oauth.RefreshAccessToken(ctx, uc.oauthCfg, providerID, refreshToken)
					if refreshErr == nil {
						chosen.Credential = newAccess
						go uc.persistRefreshedToken(chosen.ID, newAccess, newRefresh, expiresIn)
						logger.L.Infow("routing: oauth token refreshed", "modelAccountID", chosen.ID)
					} else {
						logger.L.Warnw("routing: oauth token refresh failed, using existing token",
							"modelAccountID", chosen.ID, "err", refreshErr)
					}
				}
			}
		}
	}
	return nil
}

// selectModelAccountCore 是真正的选号实现。两步候选过滤：
//  1. excludeIDs 显式排除（fallback 场景）
//  2. breaker.Allow 过滤（熔断 open 的账号）；breaker 报错时保守放行，
//     避免 Redis 抖动直接断掉整条请求。
//
// 注意：breaker 过滤空了仍返 ErrNoAvailableRoute，不做"保守全放"，
// 因为此时运维意图就是让所有账号都 open。
func (uc *UseCase) selectModelAccountCore(
	ctx context.Context,
	tokenGroup, modelName, protocol string,
	excludeIDs []int64,
) (*domainRouting.RouteResult, error) {
	m, err := uc.modelRepo.FindByName(ctx, modelName)
	if err != nil {
		logger.L.Errorw("routing: find model failed", "error", err, "model", modelName)
		return nil, errcode.ErrInternal
	}
	if m == nil || !m.IsActive {
		return nil, errcode.ErrModelNotFound
	}

	accountIDs, err := uc.tgRepo.ListModelAccountIDsByName(ctx, tokenGroup)
	if err != nil {
		logger.L.Errorw("routing: list model account ids failed", "error", err, "group", tokenGroup)
		return nil, errcode.ErrInternal
	}

	allAccounts, err := uc.accountRepo.ListActiveByModelIDs(ctx, []int64{m.ID})
	if err != nil {
		logger.L.Errorw("routing: list active model accounts failed", "error", err)
		return nil, errcode.ErrInternal
	}

	idSet := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		idSet[id] = struct{}{}
	}

	excludeSet := make(map[int64]struct{}, len(excludeIDs))
	for _, id := range excludeIDs {
		excludeSet[id] = struct{}{}
	}

	var candidates []*domainModel.ModelAccount
	for _, a := range allAccounts {
		if _, ok := idSet[a.ID]; !ok {
			continue
		}
		if protocol != "" && !routingProtocolMatches(protocol, a.Protocol) {
			continue
		}
		if _, excluded := excludeSet[a.ID]; excluded {
			continue
		}
		if uc.breaker != nil {
			allowed, bErr := uc.breaker.Allow(ctx, a.ID)
			if bErr != nil {
				logger.L.Warnw("routing: circuit breaker Allow failed, allowing conservatively",
					"modelAccountID", a.ID, "error", bErr)
			} else if !allowed {
				continue
			}
		}
		candidates = append(candidates, a)
	}

	if len(candidates) == 0 {
		return nil, errcode.ErrNoAvailableRoute
	}

	chosen := weightedRandom(candidates)

	if err := uc.materializeAccountCredentials(ctx, chosen, m); err != nil {
		return nil, err
	}

	return &domainRouting.RouteResult{Account: chosen, Model: m}, nil
}

// ListAvailableModels returns distinct active models accessible from a tokenGroup.
func (uc *UseCase) ListAvailableModels(ctx context.Context, tokenGroup string) ([]*domainModel.Model, error) {
	accountIDs, err := uc.tgRepo.ListModelAccountIDsByName(ctx, tokenGroup)
	if err != nil {
		logger.L.Errorw("routing: list model account ids failed", "error", err, "group", tokenGroup)
		return nil, errcode.ErrInternal
	}
	if len(accountIDs) == 0 {
		return nil, nil
	}

	allModels, err := uc.modelRepo.List(ctx, domainModel.ListFilter{})
	if err != nil {
		logger.L.Errorw("routing: list models failed", "error", err)
		return nil, errcode.ErrInternal
	}

	modelIDSet := make(map[int64]*domainModel.Model, len(allModels))
	var activeModelIDs []int64
	for _, m := range allModels {
		if m.IsActive && m.IsListed {
			modelIDSet[m.ID] = m
			activeModelIDs = append(activeModelIDs, m.ID)
		}
	}

	accounts, err := uc.accountRepo.ListActiveByModelIDs(ctx, activeModelIDs)
	if err != nil {
		logger.L.Errorw("routing: list model accounts for models failed", "error", err)
		return nil, errcode.ErrInternal
	}

	idSet := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		idSet[id] = struct{}{}
	}

	reachableModelIDs := make(map[int64]struct{})
	for _, a := range accounts {
		if _, ok := idSet[a.ID]; ok {
			reachableModelIDs[a.ModelID] = struct{}{}
		}
	}

	var result []*domainModel.Model
	for mid := range reachableModelIDs {
		if m, ok := modelIDSet[mid]; ok {
			result = append(result, m)
		}
	}
	return result, nil
}

func (uc *UseCase) persistRefreshedToken(modelAccountID int64, newAccess, newRefresh string, expiresIn int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existing, err := uc.accountRepo.FindByID(ctx, modelAccountID)
	if err != nil || existing == nil {
		logger.L.Errorw("routing: persist refreshed token: find model account failed", "modelAccountID", modelAccountID, "err", err)
		return
	}

	encAccess, err := crypto.EncryptAES(newAccess, uc.aesKey)
	if err != nil {
		logger.L.Errorw("routing: encrypt new access_token failed", "err", err)
		return
	}
	existing.Credential = encAccess

	if newRefresh != "" {
		encRefresh, err := crypto.EncryptAES(newRefresh, uc.aesKey)
		if err != nil {
			logger.L.Errorw("routing: encrypt new refresh_token failed", "err", err)
			return
		}
		existing.RefreshToken = encRefresh
	}

	if expiresIn > 0 {
		t := time.Now().Add(time.Duration(expiresIn) * time.Second)
		existing.TokenExpiresAt = &t
	}

	if err := uc.accountRepo.Update(ctx, existing); err != nil {
		logger.L.Errorw("routing: persist refreshed token failed", "modelAccountID", modelAccountID, "err", err)
	}
}

// routingProtocolMatches 判断上游账号协议是否可用于当前网关入口。
// 控制台创建模型账号时默认 protocol 曾为 "chat"（OpenAI Chat Completions 兼容），
// 而数据面 /v1/chat/completions 路由请求 protocol "openai"，二者应对齐。
// 数据面 /v1/embeddings 使用 protocol "embeddings"，与同样走 OpenAI 兼容上游的 "chat" 账号兼容。
// 数据面 /v1/images/generations 与 /v1/images/edits 使用 protocol "openai"，与 "image" 账号兼容。
func routingProtocolMatches(requested, account string) bool {
	if account == requested {
		return true
	}
	if requested == "openai" && account == "chat" {
		return true
	}
	if requested == "openai" && account == "image" {
		return true
	}
	if requested == "embeddings" && account == "chat" {
		return true
	}
	return false
}

func weightedRandom(accounts []*domainModel.ModelAccount) *domainModel.ModelAccount {
	totalWeight := 0
	for _, a := range accounts {
		totalWeight += a.Weight
	}
	if totalWeight <= 0 {
		return accounts[rand.Intn(len(accounts))]
	}
	r := rand.Intn(totalWeight)
	for _, a := range accounts {
		r -= a.Weight
		if r < 0 {
			return a
		}
	}
	return accounts[len(accounts)-1]
}
