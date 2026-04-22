package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	apprl "github.com/trailyai/traffic-ai/internal/application/ratelimit"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/dto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type RateLimitHandler struct {
	useCase *apprl.UseCase
}

func NewRateLimitHandler(useCase *apprl.UseCase) *RateLimitHandler {
	return &RateLimitHandler{useCase: useCase}
}

func (h *RateLimitHandler) Register(group *gin.RouterGroup) {
	group.GET("/rate-limits", h.List)
	group.POST("/rate-limits", h.Create)
	group.PUT("/rate-limits/:id", h.Update)
	group.DELETE("/rate-limits/:id", h.Delete)
}

func (h *RateLimitHandler) List(c *gin.Context) {
	rules, err := h.useCase.List(c.Request.Context())
	if err != nil {
		if appErr, ok := err.(*errcode.AppError); ok {
			response.Fail(c, appErr)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}
	response.OK(c, dto.ToRateLimitRuleList(rules))
}

func (h *RateLimitHandler) Create(c *gin.Context) {
	var req dto.CreateRateLimitRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, 400, errcode.ErrBadRequest.Code, err.Error())
		return
	}
	rule := req.ToDomain()
	if err := h.useCase.Create(c.Request.Context(), rule); err != nil {
		if appErr, ok := err.(*errcode.AppError); ok {
			response.Fail(c, appErr)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}
	response.OK(c, dto.ToRateLimitRuleItem(rule))
}

func (h *RateLimitHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	var req dto.UpdateRateLimitRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, 400, errcode.ErrBadRequest.Code, err.Error())
		return
	}
	rule := req.ToDomain(id)
	if err := h.useCase.Update(c.Request.Context(), rule); err != nil {
		if appErr, ok := err.(*errcode.AppError); ok {
			response.Fail(c, appErr)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}
	response.OK(c, dto.ToRateLimitRuleItem(rule))
}

func (h *RateLimitHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	if err := h.useCase.Delete(c.Request.Context(), id); err != nil {
		if appErr, ok := err.(*errcode.AppError); ok {
			response.Fail(c, appErr)
			return
		}
		response.Fail(c, errcode.ErrInternal)
		return
	}
	response.OKMsg(c, "deleted")
}
