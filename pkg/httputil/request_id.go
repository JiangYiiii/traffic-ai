// Package httputil 提供 HTTP 工具函数，如 RequestID 生成与传递。
package httputil

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDKey = "X-Request-Id"

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDKey)
		if rid == "" {
			rid = uuid.New().String()
		}
		c.Set(RequestIDKey, rid)
		c.Header(RequestIDKey, rid)
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	v, _ := c.Get(RequestIDKey)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
