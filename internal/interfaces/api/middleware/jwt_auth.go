package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/jwt"
	"github.com/trailyai/traffic-ai/pkg/response"
)

func JWTAuth(jwtMgr *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			response.Fail(c, errcode.ErrUnauthorized)
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := jwtMgr.ParseAccess(tokenStr)
		if err != nil {
			response.Fail(c, errcode.ErrUnauthorized)
			c.Abort()
			return
		}

		c.Set("uid", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
