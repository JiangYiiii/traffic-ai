package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/model"
	"github.com/trailyai/traffic-ai/internal/domain/provider"
	"github.com/trailyai/traffic-ai/internal/pkg/upstreamurl"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

type UseCase struct {
	modelRepo   domain.ModelRepository
	accountRepo domain.ModelAccountRepository
	aesKey      []byte
}

func NewUseCase(modelRepo domain.ModelRepository, accountRepo domain.ModelAccountRepository, aesKey []byte) *UseCase {
	return &UseCase{modelRepo: modelRepo, accountRepo: accountRepo, aesKey: aesKey}
}

// CreateModelOpts 创建模型时可选同时创建一条默认账号（如填写了密钥）。
type CreateModelOpts struct {
	AccountCredential string
	AccountEndpoint   string
}

// ---- Model CRUD ----

func (uc *UseCase) CreateModel(ctx context.Context, m *domain.Model, opts *CreateModelOpts) error {
	existing, err := uc.modelRepo.FindByName(ctx, m.ModelName)
	if err != nil {
		logger.L.Errorw("find model by name failed", "error", err)
		return errcode.ErrInternal
	}
	if existing != nil {
		return errcode.ErrDuplicateModel
	}
	if err := uc.modelRepo.Create(ctx, m); err != nil {
		logger.L.Errorw("create model failed", "error", err)
		return errcode.ErrInternal
	}
	if opts != nil && strings.TrimSpace(opts.AccountCredential) != "" {
		ep, err := resolveEndpointForAccountCreate(m, opts.AccountEndpoint)
		if err != nil {
			_ = uc.modelRepo.Delete(ctx, m.ID)
			return err
		}
		a := &domain.ModelAccount{
			ModelID:    m.ID,
			Provider:   m.Provider,
			Name:       "default",
			Endpoint:   ep,
			Credential: strings.TrimSpace(opts.AccountCredential),
			AuthType:   "api_key",
			Protocol:   "chat",
			Weight:     1,
			TimeoutSec: 60,
			IsActive:   true,
		}
		if err := uc.CreateModelAccount(ctx, a); err != nil {
			_ = uc.modelRepo.Delete(ctx, m.ID)
			return err
		}
	}
	return nil
}

func resolveEndpointForAccountCreate(m *domain.Model, override string) (string, error) {
	override = strings.TrimSpace(override)
	if def, ok := provider.ResolveDefinition(m.Provider); ok {
		if def.RequireBaseURL {
			if override == "" {
				return "", errcode.ErrModelAccountEndpointRequired
			}
			return strings.TrimRight(override, "/"), nil
		}
		if override != "" {
			return strings.TrimRight(override, "/"), nil
		}
		return provider.StoredChatEndpoint(def), nil
	}
	if override == "" {
		return "", errcode.ErrModelAccountEndpointRequired
	}
	return strings.TrimRight(override, "/"), nil
}

func (uc *UseCase) ListModels(ctx context.Context, filter domain.ListFilter) ([]*domain.Model, error) {
	models, err := uc.modelRepo.List(ctx, filter)
	if err != nil {
		logger.L.Errorw("list models failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return models, nil
}

// GetListedModels 获取已上架的模型列表 (is_active=1 AND is_listed=1)
func (uc *UseCase) GetListedModels(ctx context.Context) ([]*domain.Model, error) {
	models, err := uc.modelRepo.ListListedModels(ctx)
	if err != nil {
		logger.L.Errorw("list listed models failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return models, nil
}

func (uc *UseCase) UpdateModel(ctx context.Context, m *domain.Model) error {
	existing, err := uc.modelRepo.FindByID(ctx, m.ID)
	if err != nil {
		logger.L.Errorw("find model by id failed", "error", err)
		return errcode.ErrInternal
	}
	if existing == nil {
		return errcode.ErrModelNotFound
	}
	if err := uc.modelRepo.Update(ctx, m); err != nil {
		logger.L.Errorw("update model failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

// BatchUpdateModels 将同一套计费字段应用到多条模型（不改模型名、提供商等）。
func (uc *UseCase) BatchUpdateModels(ctx context.Context, ids []int64, billingType string, in, out, reasoning, perReq int64) error {
	for _, id := range ids {
		existing, err := uc.modelRepo.FindByID(ctx, id)
		if err != nil {
			logger.L.Errorw("batch update find model failed", "error", err)
			return errcode.ErrInternal
		}
		if existing == nil {
			return errcode.ErrModelNotFound
		}
		if billingType != "" {
			existing.BillingType = domain.BillingType(billingType)
		}
		existing.InputPrice = in
		existing.OutputPrice = out
		existing.ReasoningPrice = reasoning
		existing.PerRequestPrice = perReq
		if err := uc.modelRepo.Update(ctx, existing); err != nil {
			logger.L.Errorw("batch update model failed", "error", err)
			return errcode.ErrInternal
		}
	}
	return nil
}

func (uc *UseCase) DeleteModel(ctx context.Context, id int64) error {
	existing, err := uc.modelRepo.FindByID(ctx, id)
	if err != nil {
		logger.L.Errorw("find model by id failed", "error", err)
		return errcode.ErrInternal
	}
	if existing == nil {
		return errcode.ErrModelNotFound
	}
	if err := uc.modelRepo.Delete(ctx, id); err != nil {
		logger.L.Errorw("delete model failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

// ---- ModelAccount CRUD ----

func (uc *UseCase) CreateModelAccount(ctx context.Context, a *domain.ModelAccount) error {
	m, err := uc.modelRepo.FindByID(ctx, a.ModelID)
	if err != nil {
		logger.L.Errorw("find model for account failed", "error", err)
		return errcode.ErrInternal
	}
	if m == nil {
		return errcode.ErrModelNotFound
	}

	if a.Provider == "" {
		a.Provider = m.Provider
	}
	if strings.TrimSpace(a.Endpoint) == "" {
		return errcode.ErrModelAccountEndpointRequired
	}
	if strings.TrimSpace(a.Credential) == "" {
		return errcode.ErrModelAccountCredentialRequired
	}

	encrypted, err := crypto.EncryptAES(a.Credential, uc.aesKey)
	if err != nil {
		logger.L.Errorw("encrypt credential failed", "error", err)
		return errcode.ErrInternal
	}
	a.Credential = encrypted

	if err := uc.accountRepo.Create(ctx, a); err != nil {
		logger.L.Errorw("create model account failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

// FindModelAccount 查单条账号（不解密凭证，用于快捷操作读取再修改）。
func (uc *UseCase) FindModelAccount(ctx context.Context, id int64) (*domain.ModelAccount, error) {
	a, err := uc.accountRepo.FindByID(ctx, id)
	if err != nil {
		logger.L.Errorw("find model account by id failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return a, nil
}

func (uc *UseCase) ListModelAccounts(ctx context.Context, modelID int64) ([]*domain.ModelAccount, error) {
	list, err := uc.accountRepo.ListByModelID(ctx, modelID)
	if err != nil {
		logger.L.Errorw("list model accounts failed", "error", err)
		return nil, errcode.ErrInternal
	}
	for _, a := range list {
		plain, err := crypto.DecryptAES(a.Credential, uc.aesKey)
		if err != nil {
			logger.L.Errorw("decrypt credential failed", "error", err, "modelAccountID", a.ID)
			a.Credential = ""
			continue
		}
		a.Credential = plain
	}
	return list, nil
}

func (uc *UseCase) UpdateModelAccount(ctx context.Context, a *domain.ModelAccount) error {
	existing, err := uc.accountRepo.FindByID(ctx, a.ID)
	if err != nil {
		logger.L.Errorw("find model account by id failed", "error", err)
		return errcode.ErrInternal
	}
	if existing == nil {
		return errcode.ErrModelAccountNotFound
	}

	a.ModelID = existing.ModelID
	if a.Provider == "" {
		a.Provider = existing.Provider
	}
	if a.AuthType == "" {
		a.AuthType = existing.AuthType
	}
	if a.Protocol == "" {
		a.Protocol = existing.Protocol
	}
	if a.RefreshToken == "" {
		a.RefreshToken = existing.RefreshToken
	}
	if a.TokenExpiresAt == nil && existing.TokenExpiresAt != nil {
		a.TokenExpiresAt = existing.TokenExpiresAt
	}

	if a.Credential != "" {
		encrypted, err := crypto.EncryptAES(a.Credential, uc.aesKey)
		if err != nil {
			logger.L.Errorw("encrypt credential failed", "error", err)
			return errcode.ErrInternal
		}
		a.Credential = encrypted
	} else {
		a.Credential = existing.Credential
	}

	if err := uc.accountRepo.Update(ctx, a); err != nil {
		logger.L.Errorw("update model account failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) DeleteModelAccount(ctx context.Context, id int64) error {
	existing, err := uc.accountRepo.FindByID(ctx, id)
	if err != nil {
		logger.L.Errorw("find model account by id failed", "error", err)
		return errcode.ErrInternal
	}
	if existing == nil {
		return errcode.ErrModelAccountNotFound
	}
	if err := uc.accountRepo.Delete(ctx, id); err != nil {
		logger.L.Errorw("delete model account failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

// ---- Connectivity Test ----

type TestResult struct {
	Success   bool   `json:"success"`
	LatencyMs int    `json:"latency_ms"`
	Model     string `json:"model"`
	Account   string `json:"account"`
	Error     string `json:"error,omitempty"`
}

// connectivityProbeUsesEmbeddings 控制台连通性探测是否应走 OpenAI 兼容 /embeddings。
// 与数据面路由一致：model_type=embedding 的模型，或账号 protocol=embeddings。
func connectivityProbeUsesEmbeddings(m *domain.Model, account *domain.ModelAccount) bool {
	if strings.EqualFold(m.ModelType, "embedding") {
		return true
	}
	if strings.EqualFold(account.Protocol, "embeddings") {
		return true
	}
	return false
}

// testModelAccountHTTP 对指定模型与账号发起一次最小探测请求（不落库）。
// - 普通模型：POST …/chat/completions
// - 向量模型（model_type=embedding）或 embeddings 协议账号：POST …/embeddings
func (uc *UseCase) testModelAccountHTTP(ctx context.Context, m *domain.Model, target *domain.ModelAccount) *TestResult {
	result := &TestResult{
		Model:   m.ModelName,
		Account: target.Name,
	}
	if result.Account == "" {
		result.Account = target.Endpoint
	}

	plain, err := crypto.DecryptAES(target.Credential, uc.aesKey)
	if err != nil {
		logger.L.Errorw("test connectivity: decrypt credential failed", "error", err)
		result.Error = "账号密钥解密失败：请展开该模型，编辑对应账号并重新保存 API Key"
		return result
	}

	var reqBody []byte
	var urlStr string
	if connectivityProbeUsesEmbeddings(m, target) {
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model": m.ModelName,
			"input": "traffic connectivity probe",
		})
		urlStr = upstreamurl.JoinPath(target.Endpoint, "/embeddings")
	} else {
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":                 m.ModelName,
			"messages":              []map[string]string{{"role": "user", "content": "Hi"}},
			"max_completion_tokens": 5,
		})
		urlStr = upstreamurl.JoinPath(target.Endpoint, "/chat/completions")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(reqBody))
	if err != nil {
		result.Error = fmt.Sprintf("build request: %v", err)
		return result
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+plain)

	client := &http.Client{Timeout: 15 * time.Second}
	start := time.Now()
	resp, err := client.Do(httpReq)
	result.LatencyMs = int(time.Since(start).Milliseconds())

	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
	} else {
		logger.L.Errorw("test connectivity failed",
			"url", urlStr,
			"model", m.ModelName,
			"status", resp.StatusCode,
			"response", string(body))

		var apiError struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}

		if json.Unmarshal(body, &apiError) == nil && apiError.Error.Message != "" {
			result.Error = fmt.Sprintf("OpenAI API错误 (%s): %s", apiError.Error.Type, apiError.Error.Message)
		} else {
			result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncateString(string(body), 200))
		}
	}
	return result
}

func (uc *UseCase) TestModelConnectivity(ctx context.Context, modelID int64) (*TestResult, error) {
	m, err := uc.modelRepo.FindByID(ctx, modelID)
	if err != nil {
		logger.L.Errorw("test connectivity: find model failed", "error", err)
		return nil, errcode.ErrInternal
	}
	if m == nil {
		return nil, errcode.ErrModelNotFound
	}

	accounts, err := uc.accountRepo.ListByModelID(ctx, modelID)
	if err != nil {
		logger.L.Errorw("test connectivity: list model accounts failed", "error", err)
		return nil, errcode.ErrInternal
	}

	var target *domain.ModelAccount
	for _, a := range accounts {
		if a.IsActive {
			target = a
			break
		}
	}
	if target == nil {
		return nil, errcode.ErrNoAvailableRoute
	}

	result := uc.testModelAccountHTTP(ctx, m, target)
	uc.persistTestOutcome(ctx, m.ID, result)
	return result, nil
}

// TestModelAccountConnectivity 针对单条 model_account 做连通性探测，并写入该账号的 last_test_*。
func (uc *UseCase) TestModelAccountConnectivity(ctx context.Context, accountID int64) (*TestResult, error) {
	target, err := uc.accountRepo.FindByID(ctx, accountID)
	if err != nil {
		logger.L.Errorw("test model account: find account failed", "error", err)
		return nil, errcode.ErrInternal
	}
	if target == nil {
		return nil, errcode.ErrModelAccountNotFound
	}
	m, err := uc.modelRepo.FindByID(ctx, target.ModelID)
	if err != nil {
		logger.L.Errorw("test model account: find model failed", "error", err)
		return nil, errcode.ErrInternal
	}
	if m == nil {
		return nil, errcode.ErrModelNotFound
	}

	result := uc.testModelAccountHTTP(ctx, m, target)
	uc.persistAccountTestOutcome(ctx, target.ID, result)
	return result, nil
}

func (uc *UseCase) persistTestOutcome(ctx context.Context, modelID int64, r *TestResult) {
	if r == nil {
		return
	}
	errMsg := ""
	if !r.Success {
		errMsg = truncateString(r.Error, 500)
	}
	if err := uc.modelRepo.UpdateLastTest(ctx, modelID, r.Success, r.LatencyMs, errMsg); err != nil {
		logger.L.Warnw("persist test outcome failed", "error", err, "modelID", modelID)
	}
}

func (uc *UseCase) persistAccountTestOutcome(ctx context.Context, accountID int64, r *TestResult) {
	if r == nil {
		return
	}
	errMsg := ""
	if !r.Success {
		errMsg = truncateString(r.Error, 500)
	}
	if err := uc.accountRepo.UpdateLastTest(ctx, accountID, r.Success, r.LatencyMs, errMsg); err != nil {
		logger.L.Warnw("persist account test outcome failed", "error", err, "accountID", accountID)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
