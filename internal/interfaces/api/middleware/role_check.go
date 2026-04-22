package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

// RequireAdmin 客户管理端权限：admin 或 super_admin 均可通过。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Fail(c, errcode.ErrUnauthorized)
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || (roleStr != "admin" && roleStr != "super_admin") {
			response.Fail(c, errcode.ErrForbidden)
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireSuperAdmin 模型管理端权限：仅 super_admin 可通过。
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Fail(c, errcode.ErrUnauthorized)
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || roleStr != "super_admin" {
			response.Fail(c, errcode.ErrForbidden)
			c.Abort()
			return
		}

		c.Next()
	}
}
