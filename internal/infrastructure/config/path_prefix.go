package config

import "strings"

// NormalizePathPrefix 规范化 HTTP 路径前缀：非空时以 / 开头、无尾部 /；空或 "/" 视为无前缀。
func NormalizePathPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

// NormalizedControlPathPrefix 返回控制面路径前缀（已规范化）。
func (s ServerConfig) NormalizedControlPathPrefix() string {
	return NormalizePathPrefix(s.ControlPathPrefix)
}

// NormalizedGatewayPathPrefix 返回网关路径前缀（已规范化）。
func (s ServerConfig) NormalizedGatewayPathPrefix() string {
	return NormalizePathPrefix(s.GatewayPathPrefix)
}

// OAuthCallbackPath 返回 OAuth 回调相对路径（含控制面前缀）。
func (s ServerConfig) OAuthCallbackPath() string {
	return s.NormalizedControlPathPrefix() + "/admin/oauth/callback"
}
