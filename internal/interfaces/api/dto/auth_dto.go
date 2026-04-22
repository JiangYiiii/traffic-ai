// Package dto 认证相关请求/响应数据传输对象。
package dto

// @ai_doc SendCodeReq: 发送验证码请求(注册/重置密码共用)
type SendCodeReq struct {
	Email string `json:"email" binding:"required,email"`
}

// @ai_doc RegisterReq: 注册请求，需携带验证码
type RegisterReq struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=64"`
	Code     string `json:"code"     binding:"required,len=6"`
}

// @ai_doc LoginReq: 邮箱密码登录
type LoginReq struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// @ai_doc RefreshReq: 刷新令牌请求
type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// @ai_doc ResetPasswordReq: 密码重置请求
type ResetPasswordReq struct {
	Email       string `json:"email"        binding:"required,email"`
	Code        string `json:"code"         binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=64"`
}

// @ai_doc TokenResp: JWT 令牌对响应
type TokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
