package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
)

// trafficConfigHandler 向前端注入运行时路径前缀与端口信息。
func trafficConfigHandler(cfg *config.Config) gin.HandlerFunc {
	controlPrefix := cfg.Server.NormalizedControlPathPrefix()
	gatewayPrefix := cfg.Server.NormalizedGatewayPathPrefix()
	body := fmt.Sprintf(
		`window.__TRAFFIC_CONFIG__={controlPath:%q,gatewayPath:%q,gatewayPort:%q,userPort:%q,adminPort:%q};`,
		controlPrefix,
		gatewayPrefix,
		fmt.Sprintf("%d", cfg.Server.GatewayPort),
		fmt.Sprintf("%d", cfg.Server.ControlPort),
		fmt.Sprintf("%d", cfg.Server.AdminControlPort),
	)
	return func(c *gin.Context) {
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-store, max-age=0")
		c.String(http.StatusOK, body)
	}
}

func redirectWithPrefix(prefix, target string) gin.HandlerFunc {
	dest := prefix + target
	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, dest)
	}
}

func mountRootRedirect(r *gin.Engine, prefix, target string) {
	if prefix == "" {
		return
	}
	r.GET("/", redirectWithPrefix(prefix, target))
}
