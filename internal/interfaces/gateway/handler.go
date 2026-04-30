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
	}

	items := make([]modelItem, 0, len(models))
	for _, m := range models {
		items = append(items, modelItem{
			ID:      m.ModelName,
			Object:  "model",
			Created: m.CreatedAt.Unix(),
			OwnedBy: m.Provider,
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
