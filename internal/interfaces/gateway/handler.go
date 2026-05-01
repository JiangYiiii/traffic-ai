package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	gwuc "github.com/trailyai/traffic-ai/internal/application/gateway"
	"github.com/trailyai/traffic-ai/internal/domain/token"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/httputil"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type Handler struct {
	uc *gwuc.UseCase
}

func NewHandler(uc *gwuc.UseCase) *Handler {
	return &Handler{uc: uc}
}

// ListModels GET /v1/models — OpenAI 兼容格式。
func (h *Handler) ListModels(c *gin.Context) {
	tok := mustGetToken(c)

	models, err := h.uc.ListModels(c.Request.Context(), tok.TokenGroup)
	if err != nil {
		failWithErr(c, err)
		return
	}

	type modelItem struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
		// Traffic 扩展字段：chat | embedding | image | speech 等，便于调用端（如 OpenClaw 同步脚本）
		// 区分「对话模型」与「仅图片生成」部署；OpenAI 官方模型对象外的附加键，兼容客户端可忽略。
		TrafficModelType string `json:"traffic_model_type,omitempty"`
	}

	items := make([]modelItem, 0, len(models))
	for _, m := range models {
		items = append(items, modelItem{
			ID:               m.ModelName,
			Object:           "model",
			Created:          m.CreatedAt.Unix(),
			OwnedBy:          m.Provider,
			TrafficModelType: m.ModelType,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   items,
	})
}

// ChatCompletions POST /v1/chat/completions — 支持非流式和流式 SSE。
func (h *Handler) ChatCompletions(c *gin.Context) {
	tok := mustGetToken(c)
	requestID := httputil.GetRequestID(c)

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024))
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	defer c.Request.Body.Close()

	written, proxyErr := h.uc.ChatCompletions(
		c.Request.Context(),
		tok,
		body,
		c.Request.Header,
		c.Writer,
		requestID,
		c.ClientIP(),
	)
	if proxyErr != nil {
		if written {
			return
		}
		failWithErr(c, proxyErr)
	}
}

// ImagesGenerations POST /v1/images/generations — OpenAI / Azure OpenAI Images API。
func (h *Handler) ImagesGenerations(c *gin.Context) {
	h.genericProxy(c, "openai", "/images/generations")
}

// ImagesEdits POST /v1/images/edits — multipart 透传（参考图 / mask 编辑）；表单字段 model 用于路由。
func (h *Handler) ImagesEdits(c *gin.Context) {
	tok := mustGetToken(c)
	requestID := httputil.GetRequestID(c)

	ct := strings.TrimSpace(c.GetHeader("Content-Type"))
	if ct == "" || !strings.HasPrefix(strings.ToLower(ct), "multipart/") {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, gwuc.MaxImageEditsRequestBody))
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	defer c.Request.Body.Close()

	written, proxyErr := h.uc.ProxyImageEdits(
		c.Request.Context(),
		tok,
		body,
		ct,
		c.Request.Header,
		c.Writer,
		requestID,
		c.ClientIP(),
	)
	if proxyErr != nil {
		if written {
			return
		}
		failWithErr(c, proxyErr)
	}
}

func filesModelHint(c *gin.Context) string {
	return strings.TrimSpace(c.Query("model"))
}

func openAIFilePathSegment(id string) bool {
	if len(id) == 0 || len(id) > 200 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// FilesCreate POST /v1/files — multipart；路由用可选查询参数 model（缺省时在 tokenGroup 内任选 OpenAI 兼容账号）。
func (h *Handler) FilesCreate(c *gin.Context) {
	tok := mustGetToken(c)
	requestID := httputil.GetRequestID(c)
	ct := strings.TrimSpace(c.GetHeader("Content-Type"))
	if ct == "" || !strings.HasPrefix(strings.ToLower(ct), "multipart/") {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, gwuc.MaxFileUploadRequestBody))
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	defer c.Request.Body.Close()
	written, proxyErr := h.uc.ProxyOpenAIFiles(
		c.Request.Context(), tok, http.MethodPost, "/files", c.Request.URL.RawQuery,
		body, ct, false, c.Request.Header, c.Writer, filesModelHint(c), requestID, c.ClientIP(),
	)
	if proxyErr != nil {
		if written {
			return
		}
		failWithErr(c, proxyErr)
	}
}

// FilesList GET /v1/files
func (h *Handler) FilesList(c *gin.Context) {
	h.proxyFilesRead(c, http.MethodGet, "/files", false)
}

// FilesRetrieve GET /v1/files/:file_id
func (h *Handler) FilesRetrieve(c *gin.Context) {
	id := c.Param("file_id")
	if !openAIFilePathSegment(id) {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	h.proxyFilesRead(c, http.MethodGet, "/files/"+id, false)
}

// FilesDownloadContent GET /v1/files/:file_id/content
func (h *Handler) FilesDownloadContent(c *gin.Context) {
	id := c.Param("file_id")
	if !openAIFilePathSegment(id) {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	h.proxyFilesRead(c, http.MethodGet, "/files/"+id+"/content", true)
}

// FilesDelete DELETE /v1/files/:file_id
func (h *Handler) FilesDelete(c *gin.Context) {
	id := c.Param("file_id")
	if !openAIFilePathSegment(id) {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	h.proxyFilesRead(c, http.MethodDelete, "/files/"+id, false)
}

func (h *Handler) proxyFilesRead(c *gin.Context, method, pathSuffix string, binary bool) {
	tok := mustGetToken(c)
	requestID := httputil.GetRequestID(c)
	written, proxyErr := h.uc.ProxyOpenAIFiles(
		c.Request.Context(), tok, method, pathSuffix, c.Request.URL.RawQuery,
		nil, "", binary, c.Request.Header, c.Writer, filesModelHint(c), requestID, c.ClientIP(),
	)
	if proxyErr != nil {
		if written {
			return
		}
		failWithErr(c, proxyErr)
	}
}

// Embeddings POST /v1/embeddings
func (h *Handler) Embeddings(c *gin.Context) {
	h.genericProxy(c, "embeddings", "/embeddings")
}

// Responses POST /v1/responses — OpenAI Responses API。
func (h *Handler) Responses(c *gin.Context) {
	h.genericProxy(c, "openai", "/responses")
}

// AudioSpeech POST /v1/audio/speech — TTS。
func (h *Handler) AudioSpeech(c *gin.Context) {
	h.genericProxy(c, "openai", "/audio/speech")
}

// AnthropicMessages POST /v1/messages — Anthropic Messages API。
func (h *Handler) AnthropicMessages(c *gin.Context) {
	h.genericProxy(c, "anthropic", "/messages")
}

// GeminiGenerateContent POST /v1beta/models/:modelAction — Gemini generateContent / streamGenerateContent。
func (h *Handler) GeminiGenerateContent(c *gin.Context) {
	modelAction := c.Param("modelAction")
	parts := strings.SplitN(modelAction, ":", 2)
	if len(parts) != 2 {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	modelName := parts[0]
	action := parts[1]

	tok := mustGetToken(c)
	requestID := httputil.GetRequestID(c)
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024))
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	defer c.Request.Body.Close()

	isStream := strings.HasPrefix(action, "streamGenerateContent")
	upstreamPath := "/v1beta/models/" + modelAction
	if q := c.Request.URL.RawQuery; q != "" {
		upstreamPath += "?" + q
	}

	written, proxyErr := h.uc.ProxyGeneric(
		c.Request.Context(), tok, body, c.Request.Header, c.Writer, requestID,
		"gemini", upstreamPath, modelName, isStream, c.ClientIP(),
	)
	if proxyErr != nil && !written {
		failWithErr(c, proxyErr)
	}
}

// genericProxy 通用协议转发辅助方法。
func (h *Handler) genericProxy(c *gin.Context, protocol, upstreamPath string) {
	tok := mustGetToken(c)
	requestID := httputil.GetRequestID(c)

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024*1024))
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	defer c.Request.Body.Close()

	var req gwuc.GenericRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	written, proxyErr := h.uc.ProxyGeneric(
		c.Request.Context(), tok, body, c.Request.Header, c.Writer, requestID,
		protocol, upstreamPath, req.Model, req.Stream, c.ClientIP(),
	)
	if proxyErr != nil && !written {
		failWithErr(c, proxyErr)
	}
}

func mustGetToken(c *gin.Context) *token.Token {
	v, _ := c.Get(tokenCtxKey)
	return v.(*token.Token)
}

func failWithErr(c *gin.Context, err error) {
	if appErr, ok := err.(*errcode.AppError); ok {
		response.Fail(c, appErr)
		return
	}
	response.Fail(c, errcode.ErrInternal)
}

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
