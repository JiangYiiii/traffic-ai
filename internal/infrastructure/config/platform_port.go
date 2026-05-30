package config

import (
	"fmt"
	"os"
)

// applyPlatformPortFallback 读取云平台常见的 PORT 环境变量。
// 仅在该服务未显式设置 GATEWAY_PORT / CONTROL_PORT / ADMIN_CONTROL_PORT 时生效。
func applyPlatformPortFallback(s *ServerConfig) {
	v := os.Getenv("PORT")
	if v == "" {
		return
	}
	var p int
	if _, err := fmt.Sscanf(v, "%d", &p); err != nil || p <= 0 {
		return
	}
	if os.Getenv("GATEWAY_PORT") == "" {
		s.GatewayPort = p
	}
	if os.Getenv("CONTROL_PORT") == "" {
		s.ControlPort = p
	}
	if os.Getenv("ADMIN_CONTROL_PORT") == "" {
		s.AdminControlPort = p
	}
}
