package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	apptoken "github.com/trailyai/traffic-ai/internal/application/token"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/dto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type TokenHandler struct {
	uc *apptoken.UseCase
}

func NewTokenHandler(useCase *apptoken.UseCase) *TokenHandler {
	return &TokenHandler{uc: useCase}
}

func (h *TokenHandler) Register(group *gin.RouterGroup) {
	group.GET("/tokens", h.List)
	group.POST("/tokens", h.Create)
	group.PATCH("/tokens/:id/disable", h.Disable)
	group.PATCH("/tokens/:id/enable", h.Enable)
	group.DELETE("/tokens/:id", h.Delete)
}

func (h *TokenHandler) Create(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var req dto.CreateTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	plainKey, tok, err := h.uc.Create(c.Request.Context(), uid, req.Name, req.TokenGroup, req.ExpiresAt)
	if err != nil {
		if ae, ok := err.(*errcode.AppError); ok {
			response.Fail(c, ae)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}

	response.OK(c, dto.CreateTokenResp{
		ID:        tok.ID,
		Name:      tok.Name,
		Key:       plainKey,
		KeyPrefix: tok.KeyPrefix,
		CreatedAt: tok.CreatedAt.Format(time.RFC3339),
	})
}

func (h *TokenHandler) List(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	tokens, err := h.uc.List(c.Request.Context(), uid)
	if err != nil {
		if ae, ok := err.(*errcode.AppError); ok {
			response.Fail(c, ae)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}

	response.OK(c, dto.ToTokenItemList(tokens))
}

func (h *TokenHandler) Enable(c *gin.Context) {
	h.toggleActive(c, true)
}

func (h *TokenHandler) Disable(c *gin.Context) {
	h.toggleActive(c, false)
}

func (h *TokenHandler) Delete(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	tokenID, ok := parseTokenID(c)
	if !ok {
		return
	}

	if err := h.uc.Delete(c.Request.Context(), uid, tokenID); err != nil {
		if ae, ok := err.(*errcode.AppError); ok {
			response.Fail(c, ae)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}

	response.OKMsg(c, "deleted")
}

func (h *TokenHandler) toggleActive(c *gin.Context, active bool) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	tokenID, ok := parseTokenID(c)
	if !ok {
		return
	}

	var err error
	if active {
		err = h.uc.Enable(c.Request.Context(), uid, tokenID)
	} else {
		err = h.uc.Disable(c.Request.Context(), uid, tokenID)
	}
	if err != nil {
		if ae, ok := err.(*errcode.AppError); ok {
			response.Fail(c, ae)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}

	response.OKMsg(c, "ok")
}

func getUserID(c *gin.Context) (int64, bool) {
	v, exists := c.Get("uid")
	if !exists {
		response.Fail(c, errcode.ErrUnauthorized)
		return 0, false
	}
	uid, ok := v.(int64)
	if !ok {
		response.Fail(c, errcode.ErrUnauthorized)
		return 0, false
	}
	return uid, true
}

func parseTokenID(c *gin.Context) (int64, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return 0, false
	}
	return id, true
}
