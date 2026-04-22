// Package gateway 网关数据面应用层：编排鉴权 → 限流 → 路由 → 转发 → 计费 → 日志。
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/trailyai/traffic-ai/internal/domain/billing"
	domainModel "github.com/trailyai/traffic-ai/internal/domain/model"
	"github.com/trailyai/traffic-ai/internal/domain/ratelimit"
	"github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/domain/token"
	"github.com/trailyai/traffic-ai/internal/infrastructure/httpclient"
	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	redisinfra "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
	"github.com/trailyai/traffic-ai/internal/pkg/upstreamurl"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

// MetricsSink 是 application 层对 Prometheus collector 的输入端口。
// 具体实现由 interfaces/gateway.*Metrics 绑定方法提供；nil 值表示不打点（保持单测/回退可用）。
// 约束：所有方法必须 nil-safe；调用端不需判空。
type MetricsSink interface {
	IncRequest(model, account, status, protocol string)
	ObserveUpstreamLatency(model, account string, seconds float64)
	IncInflight(model, account string)
	DecInflight(model, account string)
	IncRatelimitReject(scope, reason string)
	IncRetry(model, reason string)
}

// noopSink 兜底实现；UseCase.metrics 为 nil 时使用，避免所有调用点都要判空。
type noopSink struct{}

func (noopSink) IncRequest(string, string, string, string)          {}
func (noopSink) ObserveUpstreamLatency(string, string, float64)     {}
func (noopSink) IncInflight(string, string)                         {}
func (noopSink) DecInflight(string, string)                         {}
func (noopSink) IncRatelimitReject(string, string)                  {}
func (noopSink) IncRetry(string, string)                            {}

// ChatRequest 是 OpenAI Chat Completions 请求的精简解析结构。
type ChatRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Messages json.RawMessage `json:"messages"`
}

// maxErrorMessageBytes 限制写入 usage_logs.error_message 的字节数。
// 列类型是 TEXT（64KB 上限），但应用层保守截断到 16KB：
// 既能完整容纳 Anthropic / OpenAI / 阿里云等主流上游的错误 body，
// 又避免极端上游把错误表撑爆（曾经因 VARCHAR(500) 太短，直接触发 1406 写入失败）。
const maxErrorMessageBytes = 16 * 1024

// maxUpstreamErrorBodyBytes 从上游错误响应中最多读取的字节数，用于透传给客户端并记日志。
// 够装 Anthropic/OpenAI 的 rate_limit_error、400 错误、5xx HTML 错误页等常见情况。
const maxUpstreamErrorBodyBytes = 64 * 1024

// ProxyResult 包含单次转发的完整结果信息。
type ProxyResult struct {
	StatusCode          int
	InputTokens         int
	OutputTokens        int
	ReasoningTokens     int
	TotalTokens         int
	CacheCreationTokens int
	CacheReadTokens     int
	ErrorMessage        string
}

// callContext 贯穿单次转发的“请求级”上下文：协议、客户端 IP 等。
// 这些字段不影响路由，但影响结算日志与 usage 解析方式，因此从 handler 层一路透传到 settleAndLog。
type callContext struct {
	Protocol string
	ClientIP string
}

// UseCase 网关核心编排。
type UseCase struct {
	tokenRepo      token.Repository
	routingSvc     routing.RoutingService
	billingSvc     billing.BillingService
	rateLimiter    ratelimit.RateLimiter
	usageRepo      *mysql.UsageLogRepo
	monitorCounter *redisinfra.MonitorCounter
	httpMgr        *httpclient.Manager
	// breaker 允许为 nil；为 nil 时跳过所有 RecordSuccess/RecordFailure 上报。
	breaker routing.CircuitBreaker
	// maxAttempts 控制 ChatCompletions fallback 循环最大尝试次数（含首发）。
	// <=0 时内部自动纠正为 1（即不重试）。
	maxAttempts int
	// metrics 用于 Prometheus 指标埋点，允许为 nil（构造时会替换为 noopSink）。
	metrics MetricsSink
}

func NewUseCase(
	tokenRepo token.Repository,
	routingSvc routing.RoutingService,
	billingSvc billing.BillingService,
	rateLimiter ratelimit.RateLimiter,
	usageRepo *mysql.UsageLogRepo,
	monitorCounter *redisinfra.MonitorCounter,
	httpMgr *httpclient.Manager,
	breaker routing.CircuitBreaker,
	maxAttempts int,
	metrics MetricsSink,
) *UseCase {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if metrics == nil {
		metrics = noopSink{}
	}
	return &UseCase{
		tokenRepo:      tokenRepo,
		routingSvc:     routingSvc,
		billingSvc:     billingSvc,
		rateLimiter:    rateLimiter,
		usageRepo:      usageRepo,
		monitorCounter: monitorCounter,
		httpMgr:        httpMgr,
		breaker:        breaker,
		maxAttempts:    maxAttempts,
		metrics:        metrics,
	}
}

// accountIDStr 统一 account label 值构造，避免埋点点位到处 strconv。
func accountIDStr(id int64) string {
	return strconv.FormatInt(id, 10)
}

// recordRateLimitReject 将限流错误映射为 Prometheus 计数；非结构化错误归为 unknown。
func (uc *UseCase) recordRateLimitReject(err error, requestID string, tok *token.Token, modelName string) {
	var rlErr *ratelimit.RateLimitError
	if errors.As(err, &rlErr) && rlErr != nil {
		uc.metrics.IncRatelimitReject(string(rlErr.Scope), rlErr.Reason)
		logger.L.Warnw("rate limit rejected",
			"request_id", requestID,
			"user_id", tok.UserID,
			"api_key_id", tok.ID,
			"model", modelName,
			"scope", rlErr.Scope,
			"reason", rlErr.Reason,
		)
		return
	}
	uc.metrics.IncRatelimitReject("unknown", "unknown")
	logger.L.Warnw("rate limit rejected (unstructured)",
		"request_id", requestID,
		"user_id", tok.UserID,
		"api_key_id", tok.ID,
		"model", modelName,
		"error", err,
	)
}

// Authenticate 验证 API Key，返回 Token 实体。
func (uc *UseCase) Authenticate(ctx context.Context, rawKey string) (*token.Token, error) {
	hash := crypto.HashAPIKey(rawKey)
	tok, err := uc.tokenRepo.FindByKeyHash(ctx, hash)
	if err != nil {
		logger.L.Errorw("find token by key hash failed",
			"error", err.Error(),
		)
		return nil, errcode.ErrInternal
	}
	if tok == nil {
		return nil, errcode.ErrInvalidAPIKey
	}
	if !tok.IsActive {
		return nil, errcode.ErrAPIKeyDisabled
	}
	if tok.IsExpired() {
		return nil, errcode.ErrAPIKeyExpired
	}
	return tok, nil
}

// ListModels 返回当前 tokenGroup 可用的模型列表。
func (uc *UseCase) ListModels(ctx context.Context, tokenGroup string) ([]*domainModel.Model, error) {
	return uc.routingSvc.ListAvailableModels(ctx, tokenGroup)
}

// ChatCompletions 完整的 Chat Completions 链路。
// 返回值 written=true 表示已开始向 w 写入响应数据（流式场景），调用者不应再写错误响应。
// clientIP 用于写入 usage_logs.client_ip，便于后续按 IP 排查。
func (uc *UseCase) ChatCompletions(
	ctx context.Context,
	tok *token.Token,
	reqBody []byte,
	rawHeaders http.Header,
	w http.ResponseWriter,
	requestID string,
	clientIP string,
) (written bool, err error) {
	start := time.Now()
	callCtx := &callContext{Protocol: "openai", ClientIP: clientIP}

	var chatReq ChatRequest
	if err := json.Unmarshal(reqBody, &chatReq); err != nil {
		return false, errcode.ErrBadRequest
	}
	if chatReq.Model == "" {
		return false, errcode.ErrBadRequest
	}

	// 1. 首次路由选择（提前到限流之前，以便构造 EstimatedTokens 做 TPM 预估）
	firstRoute, err := uc.routingSvc.SelectModelAccount(ctx, tok.TokenGroup, chatReq.Model, "openai")
	if err != nil {
		return false, err
	}

	// 2. 限流（RPM / Concurrent / TPM）
	rlReq := &ratelimit.CheckRequest{
		UserID:          tok.UserID,
		APIKeyID:        tok.ID,
		Model:           chatReq.Model,
		EstimatedTokens: estimateTokens(firstRoute.Model, chatReq.Stream),
	}
	if err := uc.rateLimiter.Allow(ctx, rlReq); err != nil {
		uc.recordRateLimitReject(err, requestID, tok, chatReq.Model)
		return false, err
	}
	defer uc.rateLimiter.Release(ctx, rlReq)

	// 从限流通过开始，对 model+account 维度计 inflight；整条请求链 return 时自动 -1。
	// account 取首次路由选择的结果（fallback 后仍以首发账号维度为准，避免一次请求被计为两个账号的 inflight）。
	inflightModel := firstRoute.Model.ModelName
	inflightAccount := accountIDStr(firstRoute.Account.ID)
	uc.metrics.IncInflight(inflightModel, inflightAccount)
	defer uc.metrics.DecInflight(inflightModel, inflightAccount)

	// 3. 模型上架状态校验（仅上架模型允许调用）
	if !firstRoute.Model.IsListed {
		return false, errcode.ErrModelNotListed
	}

	// 4. 余额检查 + 预扣费（fallback 循环中不重复预扣）
	estimatedCost := uc.estimateCost(firstRoute.Model, chatReq.Stream)
	if err := uc.billingSvc.CheckBalance(ctx, tok.UserID, estimatedCost); err != nil {
		return false, err
	}
	if err := uc.billingSvc.PreDeduct(ctx, tok.UserID, estimatedCost, requestID); err != nil {
		return false, err
	}

	// 流式场景下，OpenAI 默认不在 SSE 里回 usage；此处为未显式设置 include_usage 的客户端兜底开启，
	// 确保最后一帧能带回 token 统计，usage_logs 与计费才有数据。
	if chatReq.Stream {
		reqBody = injectIncludeUsageForOpenAIStream(reqBody)
	}

	// 5. fallback 循环：最多 uc.maxAttempts 次（含首发）。
	// 关键约束：
	//   - 流式请求不进循环（第一次失败直接返错，避免已 flush 的 stream 被重试）
	//   - 每次 attempt 重建 bytes.NewReader(reqBody)，避免上次 Do 消费了 Reader
	//   - 中途失败的 resp 必须显式 Close；只有最终成功路径才 defer resp.Body.Close()
	//   - 预扣费只做一次（循环外）；settleAndLog 只在最终结局（成功/最终失败）调用，中间放弃不写 usage_logs
	maxAttempts := uc.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if chatReq.Stream {
		// 流式不重试：退化到单次。
		maxAttempts = 1
	}

	route := firstRoute
	tried := make(map[int64]bool, maxAttempts)
	var lastErr error = errcode.ErrUpstreamError

	for attempt := 0; attempt < maxAttempts; attempt++ {
		isLast := attempt == maxAttempts-1

		if attempt > 0 {
			r2, rerr := uc.routingSvc.SelectModelAccountExcluding(ctx, tok.TokenGroup, chatReq.Model, "openai", idsFromTried(tried))
			if rerr != nil {
				// 没有可用账号了：用上次的结算信息落日志并返回上次错误。
				logger.L.Warnw("fallback exhausted, no more accounts",
					"request_id", requestID,
					"user_id", tok.UserID,
					"api_key_id", tok.ID,
					"model", chatReq.Model,
					"protocol", callCtx.Protocol,
					"attempt", attempt+1,
					"error", rerr.Error(),
				)
				uc.settleAndLog(tok, route, requestID, chatReq, start, &ProxyResult{ErrorMessage: lastErr.Error()}, estimatedCost, callCtx)
				return false, lastErr
			}
			if !r2.Model.IsListed {
				uc.settleAndLog(tok, route, requestID, chatReq, start, &ProxyResult{ErrorMessage: "model not listed on fallback"}, estimatedCost, callCtx)
				return false, errcode.ErrModelNotListed
			}
			route = r2
		}

		upstreamURL := upstreamurl.JoinPath(route.Account.Endpoint, "/chat/completions")
		upstreamReq, buildErr := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
		if buildErr != nil {
			logger.L.Errorw("build upstream request failed",
				"request_id", requestID,
				"user_id", tok.UserID,
				"api_key_id", tok.ID,
				"model", chatReq.Model,
				"account_id", route.Account.ID,
				"protocol", callCtx.Protocol,
				"attempt", attempt+1,
				"error", buildErr.Error(),
			)
			uc.settleAndLog(tok, route, requestID, chatReq, start, &ProxyResult{ErrorMessage: "build request failed"}, estimatedCost, callCtx)
			return false, errcode.ErrInternal
		}
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Authorization", "Bearer "+route.Account.Credential)
		if ua := rawHeaders.Get("User-Agent"); ua != "" {
			upstreamReq.Header.Set("User-Agent", ua)
		}

		client := uc.getUpstreamClient(route.Account.ID, route.Account.TimeoutSec)
		tried[route.Account.ID] = true

		resp, doErr := client.Do(upstreamReq)
		if doErr != nil {
			kind := classifyTransportError(ctx, doErr)
			if uc.breaker != nil && kind.CountsTowardsCircuit() {
				_ = uc.breaker.RecordFailure(ctx, route.Account.ID, string(kind))
			}
			lastErr = mapTransportErrToErrcode(ctx, kind, doErr)
			if !kind.Retryable() || isLast {
				logger.L.Errorw("upstream request failed (final)",
					"request_id", requestID,
					"user_id", tok.UserID,
					"api_key_id", tok.ID,
					"model", chatReq.Model,
					"account_id", route.Account.ID,
					"protocol", callCtx.Protocol,
					"upstream_kind", string(kind),
					"attempt", attempt+1,
					"upstream", upstreamURL,
					"error", doErr.Error(),
				)
				uc.settleAndLog(tok, route, requestID, chatReq, start, &ProxyResult{ErrorMessage: doErr.Error()}, estimatedCost, callCtx)
				return false, lastErr
			}
			uc.metrics.IncRetry(chatReq.Model, string(kind))
			logger.L.Warnw("upstream transport error, will retry next account",
				"request_id", requestID,
				"user_id", tok.UserID,
				"api_key_id", tok.ID,
				"model", chatReq.Model,
				"account_id", route.Account.ID,
				"protocol", callCtx.Protocol,
				"upstream_kind", string(kind),
				"attempt", attempt+1,
				"error", doErr.Error(),
			)
			continue
		}

		// 请求发出成功，拿到 resp。
		if resp.StatusCode != http.StatusOK {
			kind := classifyStatusCode(resp.StatusCode)
			if uc.breaker != nil && kind.CountsTowardsCircuit() {
				_ = uc.breaker.RecordFailure(ctx, route.Account.ID, string(kind))
			}
			if !kind.Retryable() || isLast {
				// 不重试：按原逻辑原样透传上游错误（proxyUpstreamError 内部会 settleAndLog 并 Close body）
				return uc.proxyUpstreamError(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
			}
			uc.metrics.IncRetry(chatReq.Model, string(kind))
			logger.L.Warnw("upstream non-2xx status, will retry next account",
				"request_id", requestID,
				"user_id", tok.UserID,
				"api_key_id", tok.ID,
				"model", chatReq.Model,
				"account_id", route.Account.ID,
				"protocol", callCtx.Protocol,
				"upstream_kind", string(kind),
				"attempt", attempt+1,
				"status", resp.StatusCode,
			)
			_ = resp.Body.Close()
			lastErr = errcode.ErrUpstreamError
			continue
		}

		// 200 OK：成功路径。
		if uc.breaker != nil {
			_ = uc.breaker.RecordSuccess(ctx, route.Account.ID)
		}
		defer resp.Body.Close()
		w.Header().Set("X-Request-Id", requestID)
		w.Header().Set("X-Actual-Model", route.Model.ModelName)
		if chatReq.Stream {
			return uc.handleStream(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
		}
		return false, uc.handleNonStream(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
	}

	// 理论上走不到：每次 attempt 都会 return。保留兜底。
	uc.settleAndLog(tok, route, requestID, chatReq, start, &ProxyResult{ErrorMessage: lastErr.Error()}, estimatedCost, callCtx)
	return false, lastErr
}

// idsFromTried 把 tried map 的 key 拍扁成 slice。
// 用独立函数是为了不在 fallback 循环里到处 inline 这段样板。
func idsFromTried(tried map[int64]bool) []int64 {
	if len(tried) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(tried))
	for id := range tried {
		ids = append(ids, id)
	}
	return ids
}

// mapTransportErrToErrcode 把 transport 层 kind 映射成对外 errcode。
// 保持与老路径行为一致：timeout → ErrUpstreamTimeout；ClientCancel → ctx.Err()；其它 → ErrUpstreamError。
func mapTransportErrToErrcode(ctx context.Context, kind UpstreamErrorKind, err error) error {
	switch kind {
	case UpstreamKindTimeout:
		return errcode.ErrUpstreamTimeout
	case UpstreamKindClientCancel:
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	default:
		return errcode.ErrUpstreamError
	}
}

// proxyUpstreamError 把上游非 2xx 响应原样透传给调用方（保持原始状态码、Content-Type、body、Retry-After），
// 同时落 usage_logs 并结算预扣费。返回 (written=true, err=nil)：调用者无需再包装成 502，避免：
//  1. 客户端（如 openclaw/openai SDK）在流式请求下等不到 SSE 首帧，熬到 30s 超时后把错误当成 "502 no body"；
//  2. 真正的 rate_limit / Arrearage / invalid_request 错误被 ErrUpstreamError(502, code=30004) 吞掉，
//     SDK 无从识别，既不能快速失败也无法走 per-error 的退避策略。
func (uc *UseCase) proxyUpstreamError(
	w http.ResponseWriter,
	resp *http.Response,
	tok *token.Token,
	route *routing.RouteResult,
	requestID string,
	chatReq ChatRequest,
	start time.Time,
	preDeducted int64,
	callCtx *callContext,
) (bool, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamErrorBodyBytes))

	logger.L.Warnw("upstream returned error",
		"request_id", requestID,
		"user_id", tok.UserID,
		"api_key_id", tok.ID,
		"model", chatReq.Model,
		"account_id", route.Account.ID,
		"protocol", callCtx.Protocol,
		"status", resp.StatusCode,
		"body", string(body),
	)

	errMsg := fmt.Sprintf("upstream %d: %s", resp.StatusCode, string(body))
	if len(errMsg) > maxErrorMessageBytes {
		errMsg = errMsg[:maxErrorMessageBytes]
	}
	result := &ProxyResult{StatusCode: resp.StatusCode, ErrorMessage: errMsg}
	uc.settleAndLog(tok, route, requestID, chatReq, start, result, preDeducted, callCtx)

	// 透传关键响应头：Content-Type 让客户端按原格式解析；Retry-After 让 SDK 做退避；
	// 其余 header 一律不透传，避免 Transfer-Encoding / Content-Length 与我们的 body 冲突。
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		w.Header().Set("Retry-After", ra)
	}
	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("X-Actual-Model", route.Model.ModelName)
	w.Header().Set("X-Upstream-Status", fmt.Sprintf("%d", resp.StatusCode))

	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
	return true, nil
}

// handleNonStream 处理非流式响应。
// callCtx.Protocol 决定如何解析 token usage，避免 Anthropic/Gemini 等协议的字段被 OpenAI 模板漏掉导致 token=0。
func (uc *UseCase) handleNonStream(
	w http.ResponseWriter,
	resp *http.Response,
	tok *token.Token,
	route *routing.RouteResult,
	requestID string,
	chatReq ChatRequest,
	start time.Time,
	preDeducted int64,
	callCtx *callContext,
) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		logger.L.Errorw("read upstream response failed",
			"request_id", requestID,
			"user_id", tok.UserID,
			"api_key_id", tok.ID,
			"model", chatReq.Model,
			"account_id", route.Account.ID,
			"protocol", callCtx.Protocol,
			"error", err.Error(),
		)
		result := &ProxyResult{ErrorMessage: "read response failed"}
		uc.settleAndLog(tok, route, requestID, chatReq, start, result, preDeducted, callCtx)
		return errcode.ErrUpstreamError
	}

	parsed := extractUsageFromBody(callCtx.Protocol, body)
	result := &ProxyResult{
		StatusCode:          200,
		InputTokens:         parsed.InputTokens,
		OutputTokens:        parsed.OutputTokens,
		TotalTokens:         parsed.TotalTokens,
		ReasoningTokens:     parsed.ReasoningTokens,
		CacheCreationTokens: parsed.CacheCreationTokens,
		CacheReadTokens:     parsed.CacheReadTokens,
	}

	uc.settleAndLog(tok, route, requestID, chatReq, start, result, preDeducted, callCtx)

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("X-Actual-Model", route.Model.ModelName)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return nil
}

// handleStream 处理流式 SSE 响应，逐 chunk 透传给客户端。
// 同一个 streamUsageState 跨所有 SSE 事件累计；不同协议的解析差异由 updateStreamUsage 吸收。
func (uc *UseCase) handleStream(
	w http.ResponseWriter,
	resp *http.Response,
	tok *token.Token,
	route *routing.RouteResult,
	requestID string,
	chatReq ChatRequest,
	start time.Time,
	preDeducted int64,
	callCtx *callContext,
) (bool, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		result := &ProxyResult{ErrorMessage: "streaming not supported"}
		uc.settleAndLog(tok, route, requestID, chatReq, start, result, preDeducted, callCtx)
		return false, errcode.ErrInternal
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("X-Actual-Model", route.Model.ModelName)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	state := &streamUsageState{}
	buf := make([]byte, 0, 4096)
	reader := resp.Body

	// 帧间空闲超时：上游流式响应若在 idleTimeout 内未再推送任何数据，
	// 主动关闭 resp.Body 让 reader.Read 立刻返回错误，避免请求挂到整次 Timeout 才退出。
	idleTimeout := 60 * time.Second
	if uc.httpMgr != nil {
		if d := uc.httpMgr.StreamIdleTimeout(); d > 0 {
			idleTimeout = d
		}
	}
	idleTimer := time.AfterFunc(idleTimeout, func() {
		logger.L.Warnw("stream idle timeout, closing upstream",
			"request_id", requestID,
			"user_id", tok.UserID,
			"api_key_id", tok.ID,
			"model", chatReq.Model,
			"account_id", route.Account.ID,
			"protocol", callCtx.Protocol,
			"timeout", idleTimeout,
		)
		_ = resp.Body.Close()
	})
	defer idleTimer.Stop()

	tmp := make([]byte, 4096)
	for {
		n, readErr := reader.Read(tmp)
		if n > 0 {
			idleTimer.Reset(idleTimeout)
			buf = append(buf, tmp[:n]...)

			// 处理完整的 SSE 行
			for {
				idx := bytes.Index(buf, []byte("\n"))
				if idx < 0 {
					break
				}
				line := buf[:idx]
				buf = buf[idx+1:]

				lineStr := strings.TrimSpace(string(line))

				if strings.HasPrefix(lineStr, "data: ") {
					dataStr := strings.TrimPrefix(lineStr, "data: ")
					updateStreamUsage(callCtx.Protocol, dataStr, state)
				}

				_, _ = fmt.Fprintf(w, "%s\n", string(line))
			}
			flusher.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				logger.L.Warnw("stream read error",
					"request_id", requestID,
					"user_id", tok.UserID,
					"api_key_id", tok.ID,
					"model", chatReq.Model,
					"account_id", route.Account.ID,
					"protocol", callCtx.Protocol,
					"error", readErr.Error(),
				)
			}
			break
		}
	}

	// 写出缓冲区剩余数据
	if len(buf) > 0 {
		_, _ = w.Write(buf)
		flusher.Flush()
	}

	result := state.toProxyResult()
	result.StatusCode = 200
	uc.settleAndLog(tok, route, requestID, chatReq, start, &result, preDeducted, callCtx)
	return true, nil
}

// settleAndLog 异步结算 + 写 usage_log。
// callCtx 提供本次调用的协议与客户端 IP，用于正确计算 note / client_ip 字段。
func (uc *UseCase) settleAndLog(
	tok *token.Token,
	route *routing.RouteResult,
	requestID string,
	chatReq ChatRequest,
	start time.Time,
	result *ProxyResult,
	preDeducted int64,
	callCtx *callContext,
) {
	latencyDuration := time.Since(start)
	latencyMs := int(latencyDuration.Milliseconds())
	actualCost := uc.calculateCost(route.Model, result.InputTokens, result.OutputTokens, result.ReasoningTokens)

	status := "success"
	if result.ErrorMessage != "" {
		status = "error"
	}

	// 兜底：非 proxyUpstreamError 路径（build request failed / read response failed / err.Error() 等）
	// 也可能塞超长错误串；统一截断到列限长，避免 Data too long 1406 再次出现。
	if len(result.ErrorMessage) > maxErrorMessageBytes {
		result.ErrorMessage = result.ErrorMessage[:maxErrorMessageBytes]
	}

	// 协议优先取本次请求的协议（ChatCompletions/ProxyGeneric 传入），没有再退化到 upstream 默认协议。
	protocol := ""
	if callCtx != nil && callCtx.Protocol != "" {
		protocol = callCtx.Protocol
	} else {
		protocol = route.Account.Protocol
	}

	// Prometheus 埋点：请求总量 + 上游耗时 histogram。
	// 注意这里 latency 覆盖从 handler 收请求到结算的全链路（含 fallback + 本地序列化），
	// 与 usage_logs.latency_ms 一致，避免两套口径对不上 dashboard。
	modelLabel := route.Model.ModelName
	if modelLabel == "" {
		modelLabel = chatReq.Model
	}
	accountLabel := accountIDStr(route.Account.ID)
	uc.metrics.IncRequest(modelLabel, accountLabel, status, protocol)
	uc.metrics.ObserveUpstreamLatency(modelLabel, accountLabel, latencyDuration.Seconds())
	clientIP := ""
	if callCtx != nil {
		clientIP = callCtx.ClientIP
	}

	// note：成功场景默认空；失败场景截断 error_message 作为摘要（保留最关键的 500 字节，完整内容仍在 error_message 里）。
	note := ""
	if result.ErrorMessage != "" {
		const maxNote = 200
		msg := result.ErrorMessage
		if len(msg) > maxNote {
			msg = msg[:maxNote] + "..."
		}
		note = msg
	}

	go func() {
		ctx := context.Background()

		detail := fmt.Sprintf("model=%s input=%d output=%d reasoning=%d",
			chatReq.Model, result.InputTokens, result.OutputTokens, result.ReasoningTokens)
		if err := uc.billingSvc.Settle(ctx, tok.UserID, actualCost, preDeducted, requestID, detail); err != nil {
			logger.L.Errorw("settle billing failed",
				"request_id", requestID,
				"user_id", tok.UserID,
				"api_key_id", tok.ID,
				"model", chatReq.Model,
				"account_id", route.Account.ID,
				"protocol", protocol,
				"status", status,
				"latency_ms", latencyMs,
				"error", err.Error(),
			)
		}

		log := &mysql.UsageLog{
			RequestID:           requestID,
			UserID:              tok.UserID,
			APIKeyID:            tok.ID,
			Model:               chatReq.Model,
			ModelAccountID:      route.Account.ID,
			Protocol:            protocol,
			IsStream:            chatReq.Stream,
			Status:              status,
			ErrorMessage:        result.ErrorMessage,
			InputTokens:         result.InputTokens,
			OutputTokens:        result.OutputTokens,
			ReasoningTokens:     result.ReasoningTokens,
			TotalTokens:         result.TotalTokens,
			CacheCreationTokens: result.CacheCreationTokens,
			CacheReadTokens:     result.CacheReadTokens,
			CostMicroUSD:        actualCost,
			LatencyMs:           latencyMs,
			ClientIP:            clientIP,
			Note:                note,
			CreatedAt:           time.Now(),
		}
		if err := uc.usageRepo.Create(ctx, log); err != nil {
			logger.L.Errorw("write usage log failed",
				"request_id", requestID,
				"user_id", tok.UserID,
				"api_key_id", tok.ID,
				"model", chatReq.Model,
				"account_id", route.Account.ID,
				"protocol", protocol,
				"status", status,
				"latency_ms", latencyMs,
				"error", err.Error(),
			)
		}

		if uc.monitorCounter != nil {
			uc.monitorCounter.Record(ctx, redisinfra.MonitorEvent{
				ModelName:   chatReq.Model,
				AccountID:   route.Account.ID,
				IsError:     status == "error",
				TotalTokens: result.TotalTokens,
				LatencyMs:   latencyMs,
			})
		}
	}()
}

// GenericRequest 通用请求解析结构（从请求体中提取 model 和 stream 信息）。
type GenericRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

// ProxyGeneric 通用多协议转发，支持 embeddings / responses / messages / speech 等。
// protocol: 路由选择使用的协议标识（"openai"/"responses"/"anthropic"/"gemini"/"embeddings"/"speech"）
// upstreamPath: 上游 URL 的路径后缀（如 "/chat/completions"、"/messages"）
// extractModel: 从请求体中提取模型名的函数；不同协议 JSON 结构不同
func (uc *UseCase) ProxyGeneric(
	ctx context.Context,
	tok *token.Token,
	reqBody []byte,
	rawHeaders http.Header,
	w http.ResponseWriter,
	requestID string,
	protocol string,
	upstreamPath string,
	modelName string,
	isStream bool,
	clientIP string,
) (written bool, err error) {
	start := time.Now()
	callCtx := &callContext{Protocol: protocol, ClientIP: clientIP}

	if modelName == "" {
		return false, errcode.ErrBadRequest
	}

	chatReq := ChatRequest{Model: modelName, Stream: isStream}

	// 提前路由选择以获取 Model 做 estimateTokens（TPM 预估），
	// 不影响主体逻辑：后续 fallback/计费/转发流程保持不变。
	route, err := uc.routingSvc.SelectModelAccount(ctx, tok.TokenGroup, modelName, protocol)
	if err != nil {
		return false, err
	}

	rlReq := &ratelimit.CheckRequest{
		UserID:          tok.UserID,
		APIKeyID:        tok.ID,
		Model:           modelName,
		EstimatedTokens: estimateTokens(route.Model, isStream),
	}
	if err := uc.rateLimiter.Allow(ctx, rlReq); err != nil {
		uc.recordRateLimitReject(err, requestID, tok, modelName)
		return false, err
	}
	defer uc.rateLimiter.Release(ctx, rlReq)

	inflightModel := route.Model.ModelName
	inflightAccount := accountIDStr(route.Account.ID)
	uc.metrics.IncInflight(inflightModel, inflightAccount)
	defer uc.metrics.DecInflight(inflightModel, inflightAccount)

	// 模型上架状态校验
	if !route.Model.IsListed {
		return false, errcode.ErrModelNotListed
	}

	estimatedCost := uc.estimateCost(route.Model, isStream)
	if err := uc.billingSvc.CheckBalance(ctx, tok.UserID, estimatedCost); err != nil {
		return false, err
	}
	if err := uc.billingSvc.PreDeduct(ctx, tok.UserID, estimatedCost, requestID); err != nil {
		return false, err
	}

	upstreamURL := upstreamurl.JoinPath(route.Account.Endpoint, upstreamPath)
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		logger.L.Errorw("build upstream request failed", "error", err)
		uc.settleAndLog(tok, route, requestID, chatReq, start, &ProxyResult{ErrorMessage: "build request failed"}, estimatedCost, callCtx)
		return false, errcode.ErrInternal
	}
	upstreamReq.Header.Set("Content-Type", "application/json")

	switch protocol {
	case "anthropic":
		upstreamReq.Header.Set("x-api-key", route.Account.Credential)
		if v := rawHeaders.Get("anthropic-version"); v != "" {
			upstreamReq.Header.Set("anthropic-version", v)
		} else {
			upstreamReq.Header.Set("anthropic-version", "2023-06-01")
		}
	default:
		upstreamReq.Header.Set("Authorization", "Bearer "+route.Account.Credential)
	}

	if ua := rawHeaders.Get("User-Agent"); ua != "" {
		upstreamReq.Header.Set("User-Agent", ua)
	}

	client := uc.getUpstreamClient(route.Account.ID, route.Account.TimeoutSec)

	resp, err := client.Do(upstreamReq)
	if err != nil {
		logger.L.Errorw("upstream request failed", "error", err, "upstream", upstreamURL, "protocol", protocol)
		result := &ProxyResult{ErrorMessage: err.Error()}
		uc.settleAndLog(tok, route, requestID, chatReq, start, result, estimatedCost, callCtx)
		if ctx.Err() != nil || strings.Contains(err.Error(), "timeout") {
			return false, errcode.ErrUpstreamTimeout
		}
		return false, errcode.ErrUpstreamError
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return uc.proxyUpstreamError(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
	}

	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("X-Actual-Model", route.Model.ModelName)

	ct := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(ct, "text/event-stream") || strings.Contains(ct, "event-stream")

	if isStream && isSSE {
		return uc.handleStream(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
	}
	return false, uc.handleNonStream(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
}

// estimateTokens 返回 TPM 预扣减用 token 总数（input+output）。
// 与 estimateCost 口径一致：非 stream = 2000 + 1000 = 3000；stream = 2000 + 4000 = 6000。
// 按次计费模型（per_request）对 TPM 无意义，返回 0，在限流侧跳过 TPM 检查。
func estimateTokens(m *domainModel.Model, stream bool) int64 {
	if m == nil || m.BillingType == domainModel.BillingPerRequest {
		return 0
	}
	estimatedInput := int64(2000)
	estimatedOutput := int64(1000)
	if stream {
		estimatedOutput = 4000
	}
	return estimatedInput + estimatedOutput
}

// estimateCost 估算预扣费金额（微美元），流式按 4K output tokens 预估。
func (uc *UseCase) estimateCost(m *domainModel.Model, stream bool) int64 {
	if m.BillingType == domainModel.BillingPerRequest {
		return m.PerRequestPrice
	}
	estimatedInput := int64(2000)
	estimatedOutput := int64(1000)
	if stream {
		estimatedOutput = 4000
	}
	return (estimatedInput*m.InputPrice + estimatedOutput*m.OutputPrice) / 1_000_000
}

// calculateCost 根据实际 token 用量计算费用（微美元）。
// 价格单位: per 1M tokens (micro USD)
func (uc *UseCase) calculateCost(m *domainModel.Model, inputTokens, outputTokens, reasoningTokens int) int64 {
	if m.BillingType == domainModel.BillingPerRequest {
		return m.PerRequestPrice
	}
	inputCost := int64(inputTokens) * m.InputPrice / 1_000_000
	outputCost := int64(outputTokens) * m.OutputPrice / 1_000_000
	reasoningCost := int64(reasoningTokens) * m.ReasoningPrice / 1_000_000
	return inputCost + outputCost + reasoningCost
}

// getUpstreamClient 根据 TRAFFIC_UPSTREAM_LEGACY 决定走池化还是裸 Client。
// 紧急回退：环境变量设为 "1" 时，或 Manager 未注入（gateway.upstream.enabled=false），
// 都回退到原裸 http.Client{Timeout} 路径，保障一键回滚能力。
func (uc *UseCase) getUpstreamClient(accountID int64, timeoutSec int) *http.Client {
	if os.Getenv("TRAFFIC_UPSTREAM_LEGACY") == "1" || uc.httpMgr == nil {
		c := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
		if c.Timeout == 0 {
			c.Timeout = 120 * time.Second
		}
		return c
	}
	return uc.httpMgr.For(accountID, timeoutSec)
}
