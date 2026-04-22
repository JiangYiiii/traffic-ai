// Package errcode 统一错误码体系，所有领域模块共用。
// @ai_doc 错误码分段: 1xxxx=认证, 2xxxx=API Key, 3xxxx=路由/模型, 4xxxx=计费, 5xxxx=网关, 9xxxx=系统
package errcode

import "net/http"

type AppError struct {
	HTTPStatus int    `json:"-"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
	MessageZH  string `json:"-"`
}

func (e *AppError) Error() string { return e.Message }

func (e *AppError) Localized(lang string) string {
	if lang == "zh" && e.MessageZH != "" {
		return e.MessageZH
	}
	return e.Message
}

func New(httpStatus, code int, msg string) *AppError {
	return &AppError{HTTPStatus: httpStatus, Code: code, Message: msg}
}

func NewZH(httpStatus, code int, msg, msgZH string) *AppError {
	return &AppError{HTTPStatus: httpStatus, Code: code, Message: msg, MessageZH: msgZH}
}

// ---- 系统级 ----
var (
	ErrInternal     = NewZH(http.StatusInternalServerError, 90001, "internal server error", "服务器内部错误，请稍后重试")
	ErrBadRequest   = NewZH(http.StatusBadRequest, 90002, "bad request", "请求参数不正确")
	ErrUnauthorized = NewZH(http.StatusUnauthorized, 90003, "unauthorized", "未登录或登录已过期")
	ErrForbidden    = NewZH(http.StatusForbidden, 90004, "forbidden", "无权执行此操作")
	ErrNotFound     = NewZH(http.StatusNotFound, 90005, "not found", "资源不存在")
	ErrTooMany      = NewZH(http.StatusTooManyRequests, 90006, "too many requests", "请求过于频繁，请稍后重试")
)

// ---- 认证 10xxx ----
var (
	ErrInvalidCredentials = NewZH(http.StatusUnauthorized, 10001, "invalid email or password", "邮箱或密码不正确")
	ErrEmailExists        = NewZH(http.StatusConflict, 10002, "email already registered", "该邮箱已注册")
	ErrInvalidVerifyCode  = NewZH(http.StatusBadRequest, 10003, "invalid or expired verification code", "验证码无效或已过期")
	ErrAccountLocked      = NewZH(http.StatusForbidden, 10004, "account locked due to too many failed attempts", "账户已锁定，请稍后再试")
	ErrTokenExpired       = NewZH(http.StatusUnauthorized, 10005, "token expired", "登录已过期，请重新登录")
	ErrInvalidRefresh     = NewZH(http.StatusUnauthorized, 10006, "invalid refresh token", "刷新令牌无效")
)

// ---- API Key 20xxx ----
var (
	ErrInvalidAPIKey  = New(http.StatusUnauthorized, 20001, "invalid api key")
	ErrAPIKeyDisabled = New(http.StatusForbidden, 20002, "api key is disabled")
	ErrAPIKeyExpired  = New(http.StatusForbidden, 20003, "api key has expired")
	ErrAPIKeyLimit    = New(http.StatusForbidden, 20004, "api key limit reached")
)

// ---- 模型/路由 30xxx ----
//
// 命名说明：本系统中"一个模型的一条连接方式"统一称作 ModelAccount（模型账号）。
// 旧代码里的 Upstream 概念已于 2026-04 合并到 ModelAccount；错误码同步迁移，
// 旧变量名保留作为 alias 避免外部调用者编译失败（下方 Deprecated 区）。
var (
	ErrModelNotFound       = New(http.StatusNotFound, 30001, "model not found")
	ErrNoAvailableRoute    = New(http.StatusServiceUnavailable, 30002, "无可用账号：请在该模型下添加至少一条「已启用」的模型账号后再测试")
	ErrModelAccountTimeout = New(http.StatusGatewayTimeout, 30003, "model account request timeout")
	ErrModelAccountError   = New(http.StatusBadGateway, 30004, "model account returned an error")
	ErrDuplicateModel      = New(http.StatusConflict, 30005, "model name already exists")
	ErrModelAccountNotFound = New(http.StatusNotFound, 30006, "model account not found")
	ErrTokenGroupNotFound   = New(http.StatusNotFound, 30007, "token group not found")
	ErrDuplicateTokenGroup  = New(http.StatusConflict, 30008, "token group name already exists")
	ErrModelAccountEndpointRequired = New(http.StatusBadRequest, 30009, "请填写账号的 endpoint（自定义 Base URL），或确认提供商字段与目录一致")
	ErrModelNotListed = NewZH(http.StatusForbidden, 30010, "model is not listed for public access", "该模型未上架，暂不支持调用")
	ErrModelAccountCredentialRequired = New(http.StatusBadRequest, 30011, "请填写账号凭证（API Key / OAuth token）")
)

// ---- 账号 35xxx（预留：跨模型聚合维度，暂未启用）----
var (
	ErrDuplicateAccount = New(http.StatusConflict, 35002, "account name already exists")
)

// Deprecated: 兼容旧引用，新代码统一使用 ErrModelAccount* 变体。
// 下次大版本（或 2026Q3）删除。
var (
	ErrUpstreamTimeout          = ErrModelAccountTimeout
	ErrUpstreamError            = ErrModelAccountError
	ErrUpstreamNotFound         = ErrModelAccountNotFound
	ErrUpstreamEndpointRequired = ErrModelAccountEndpointRequired
	ErrAccountNotFound          = ErrModelAccountNotFound
	ErrAccountOffline           = New(http.StatusForbidden, 35003, "model account is offline")
)

// ---- 套餐相关错误码已移除（36xxx段保留，但功能已下线）----
// 用户现可直接使用所有已上架模型，无需购买套餐

// ---- 计费 40xxx ----
var (
	ErrInsufficientBalance = NewZH(http.StatusPaymentRequired, 40001, "insufficient balance", "余额不足")
	ErrInvalidRedeemCode   = NewZH(http.StatusBadRequest, 40002, "invalid redeem code", "兑换码无效，请检查输入")
	ErrRedeemCodeUsed      = NewZH(http.StatusBadRequest, 40003, "redeem code already used", "该兑换码已被使用")
)

// ---- OAuth 70xxx ----
var (
	ErrOAuthNotConfigured = New(http.StatusBadRequest, 70001, "该商家未配置 OAuth")
	ErrOAuthStateFailed   = New(http.StatusBadRequest, 70002, "OAuth state 无效或已过期")
	ErrOAuthTokenExchange = New(http.StatusBadGateway, 70003, "OAuth token 交换失败")
)

// ---- 网关 50xxx ----
var (
	ErrRateLimited      = New(http.StatusTooManyRequests, 50001, "rate limit exceeded")
	ErrAllModelsDown    = New(http.StatusServiceUnavailable, 50002, "all available models are currently unavailable")
	ErrStreamInterrupt  = New(http.StatusBadGateway, 50003, "stream interrupted")
	ErrQueueFull        = New(http.StatusServiceUnavailable, 50004, "request queue is full, please retry later")
)
