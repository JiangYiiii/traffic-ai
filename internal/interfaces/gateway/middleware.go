package gateway

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	gwuc "github.com/trailyai/traffic-ai/internal/application/gateway"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

const tokenCtxKey = "gateway_token"

// CORSMiddleware 允许浏览器端（用户控制台「对话测试」等）跨端口调用网关。
// 网关凭 Bearer / x-api-key / ?key= 鉴权，不使用 Cookie，允许 * 源即可。
// 必须在任何业务路由前注册，以便 OPTIONS 预检在命中路由前被拦截放行，
// 否则 Gin 默认会对未注册方法回 404 "page not found"。
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header(
			"Access-Control-Allow-Headers",
			"Authorization,Content-Type,X-Request-Id,X-Api-Key,Anthropic-Version,Anthropic-Dangerous-Direct-Browser-Access",
		)
		c.Header("Access-Control-Expose-Headers", "X-Request-Id")
		c.Header("Access-Control-Max-Age", "600")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// AuthMiddleware 从 Authorization: Bearer <key> 或 x-api-key 中提取 API Key 并验证。
// 支持 OpenAI (Bearer) 和 Anthropic (x-api-key) 两种鉴权方式。
func AuthMiddleware(uc *gwuc.UseCase) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := extractAPIKey(c)
		if rawKey == "" {
			response.Fail(c, errcode.ErrInvalidAPIKey)
			c.Abort()
			return
		}

		tok, err := uc.Authenticate(c.Request.Context(), rawKey)
		if err != nil {
			if appErr, ok := err.(*errcode.AppError); ok {
				response.Fail(c, appErr)
			} else {
				response.Fail(c, errcode.ErrInternal)
			}
			c.Abort()
			return
		}

		c.Set(tokenCtxKey, tok)
		c.Next()
	}
}

func extractAPIKey(c *gin.Context) string {
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if k := strings.TrimPrefix(auth, "Bearer "); k != "" {
			return k
		}
	}
	if k := c.GetHeader("x-api-key"); k != "" {
		return k
	}
	if k := c.Query("key"); k != "" {
		return k
	}
	return ""
}
