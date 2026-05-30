package config

import "testing"

func TestApplyPlatformPortFallback(t *testing.T) {
	t.Setenv("PORT", "80")
	t.Setenv("GATEWAY_PORT", "")
	t.Setenv("CONTROL_PORT", "")
	t.Setenv("ADMIN_CONTROL_PORT", "")

	cfg := Config{Server: ServerConfig{GatewayPort: 8081, ControlPort: 8080, AdminControlPort: 8083}}
	applyPlatformPortFallback(&cfg.Server)

	if cfg.Server.GatewayPort != 80 || cfg.Server.ControlPort != 80 || cfg.Server.AdminControlPort != 80 {
		t.Fatalf("expected all ports 80, got gateway=%d control=%d admin=%d",
			cfg.Server.GatewayPort, cfg.Server.ControlPort, cfg.Server.AdminControlPort)
	}

	t.Setenv("GATEWAY_PORT", "8081")
	cfg = Config{Server: ServerConfig{GatewayPort: 8081, ControlPort: 8080, AdminControlPort: 8083}}
	applyPlatformPortFallback(&cfg.Server)
	if cfg.Server.GatewayPort != 8081 {
		t.Fatalf("GATEWAY_PORT should win, got %d", cfg.Server.GatewayPort)
	}
}
