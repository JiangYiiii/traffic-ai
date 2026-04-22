// Package handler 认证 HTTP 处理器，负责参数绑定 → 调用 UseCase → 统一响应。
package handler

import (
	"github.com/gin-gonic/gin"
	authuc "github.com/trailyai/traffic-ai/internal/application/auth"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/dto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

// @ai_doc AuthHandler: 认证路由处理器，挂载在 /auth 路由组下
type AuthHandler struct {
	uc *authuc.UseCase
}

func NewAuthHandler(useCase *authuc.UseCase) *AuthHandler {
	return &AuthHandler{uc: useCase}
}

// @ai_doc Register(路由): 在 group 下注册全部认证端点
func (h *AuthHandler) Register(group *gin.RouterGroup) {
	group.POST("/register/send-code", h.sendRegisterCode)
	group.POST("/register", h.register)
	group.POST("/login", h.login)
	group.POST("/refresh", h.refresh)
	group.POST("/reset-password/send-code", h.sendResetCode)
	group.POST("/reset-password", h.resetPassword)
}

func (h *AuthHandler) sendRegisterCode(c *gin.Context) {
	var req dto.SendCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	if err := h.uc.SendRegisterCode(c.Request.Context(), req.Email); err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OKMsg(c, "verification code sent")
}

func (h *AuthHandler) register(c *gin.Context) {
	var req dto.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	pair, err := h.uc.Register(c.Request.Context(), req.Email, req.Password, req.Code)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OK(c, dto.TokenResp{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *AuthHandler) login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	pair, err := h.uc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OK(c, dto.TokenResp{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *AuthHandler) refresh(c *gin.Context) {
	var req dto.RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	pair, err := h.uc.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OK(c, dto.TokenResp{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *AuthHandler) sendResetCode(c *gin.Context) {
	var req dto.SendCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	if err := h.uc.SendResetCode(c.Request.Context(), req.Email); err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OKMsg(c, "verification code sent")
}

func (h *AuthHandler) resetPassword(c *gin.Context) {
	var req dto.ResetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	if err := h.uc.ResetPassword(c.Request.Context(), req.Email, req.Code, req.NewPassword); err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OKMsg(c, "password reset successfully")
}

func toAppErr(err error) *errcode.AppError {
	if appErr, ok := err.(*errcode.AppError); ok {
		return appErr
	}
	return errcode.ErrInternal
}
