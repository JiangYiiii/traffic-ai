package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/trailyai/traffic-ai/internal/domain/ratelimit"
	"github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/domain/token"
	"github.com/trailyai/traffic-ai/internal/pkg/upstreamurl"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

// MaxFileUploadRequestBody POST /v1/files 允许的 multipart 体大小（OpenAI 单文件可到数百 MB，此处取保守上限）。
const MaxFileUploadRequestBody = 128 << 20

// ProxyOpenAIFiles 转发 OpenAI Files API（/v1/files、/v1/files/{id}、/v1/files/{id}/content）。
// pathSuffix 须以 /files 开头；modelHint 非空时路由到该模型在 tokenGroup 下的账号，为空时在组内任选 OpenAI 兼容账号。
// binaryDownload 为 true 时（如 …/content）将上游 body 流式抄写到客户端，不做 JSON usage 解析。
func (uc *UseCase) ProxyOpenAIFiles(
	ctx context.Context,
	tok *token.Token,
	method string,
	pathSuffix string,
	clientRawQuery string,
	reqBody []byte,
	upstreamContentType string,
	binaryDownload bool,
	rawHeaders http.Header,
	w http.ResponseWriter,
	modelHint string,
	requestID string,
	clientIP string,
) (written bool, err error) {
	pathSuffix = strings.TrimSpace(pathSuffix)
	if pathSuffix == "" || !strings.HasPrefix(pathSuffix, "/files") {
		return false, errcode.ErrBadRequest
	}

	protocol := "openai"
	start := time.Now()
	callCtx := &callContext{Protocol: protocol, ClientIP: clientIP}

	route, err := uc.routingSvc.SelectOpenAICompatibleAccount(ctx, tok.TokenGroup, modelHint)
	if err != nil {
		return false, err
	}

	modelName := route.Model.ModelName
	chatReq := ChatRequest{Model: modelName, Stream: false}

	rlReq := &ratelimit.CheckRequest{
		UserID:          tok.UserID,
		APIKeyID:        tok.ID,
		Model:           modelName,
		EstimatedTokens: estimateTokens(route.Model, false),
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

	if !route.Model.IsListed {
		return false, errcode.ErrModelNotListed
	}

	estimatedCost := uc.estimateCost(route.Model, false)
	if err := uc.billingSvc.CheckBalance(ctx, tok.UserID, estimatedCost); err != nil {
		return false, err
	}
	if err := uc.billingSvc.PreDeduct(ctx, tok.UserID, estimatedCost, requestID); err != nil {
		return false, err
	}

	upstreamBase := upstreamurl.JoinPath(route.Account.Endpoint, pathSuffix)
	upstreamURL := upstreamurl.AppendRawQuery(upstreamBase, clientRawQuery)

	var bodyReader io.Reader
	if len(reqBody) > 0 {
		bodyReader = bytes.NewReader(reqBody)
	} else {
		bodyReader = http.NoBody
	}

	upstreamReq, err := http.NewRequestWithContext(ctx, method, upstreamURL, bodyReader)
	if err != nil {
		logger.L.Errorw("build upstream files request failed", "error", err)
		uc.settleAndLog(tok, route, requestID, chatReq, start, &ProxyResult{ErrorMessage: "build request failed"}, estimatedCost, callCtx)
		return false, errcode.ErrInternal
	}
	if upstreamContentType != "" {
		upstreamReq.Header.Set("Content-Type", upstreamContentType)
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+route.Account.Credential)
	if ua := rawHeaders.Get("User-Agent"); ua != "" {
		upstreamReq.Header.Set("User-Agent", ua)
	}

	client := uc.getUpstreamClient(route.Account.ID, route.Account.TimeoutSec)
	resp, err := client.Do(upstreamReq)
	if err != nil {
		logger.L.Errorw("upstream files request failed", "error", err, "upstream", upstreamURL)
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

	if binaryDownload {
		return false, uc.handleFileBinaryDownload(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
	}

	ct := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(ct, "text/event-stream") || strings.Contains(ct, "event-stream")
	if chatReq.Stream && isSSE {
		return uc.handleStream(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
	}
	return false, uc.handleNonStream(w, resp, tok, route, requestID, chatReq, start, estimatedCost, callCtx)
}

// handleFileBinaryDownload 将上游二进制 body 流式写入客户端（如文件下载），不做 JSON usage 解析。
func (uc *UseCase) handleFileBinaryDownload(
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
	result := &ProxyResult{StatusCode: http.StatusOK}
	uc.settleAndLog(tok, route, requestID, chatReq, start, result, preDeducted, callCtx)

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	for _, h := range []string{"Content-Length", "Content-Disposition", "Cache-Control", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("X-Actual-Model", route.Model.ModelName)
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, resp.Body); err != nil {
		logger.L.Warnw("files binary response copy failed",
			"request_id", requestID, "error", err)
		return errcode.ErrUpstreamError
	}
	return nil
}
