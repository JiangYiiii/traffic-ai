package api

import (
	"io/fs"
	"net/http"
	"strings"
)

type staticScope int

const (
	staticScopeUser staticScope = iota
	staticScopeAdmin
)

// newScopedStaticHandler 按平面过滤可访问的静态资源（用户平面与管理平面分端口）。
// 资源加上缓存控制头：HTML 入口页 no-store 防止升级后浏览器仍命中旧版；
// JS / CSS 用 no-cache 走协商缓存（embed.FS 不提供 mtime，这里靠关闭强缓存保证 UI 切换立刻生效）。
func newScopedStaticHandler(root fs.FS, scope staticScope) http.Handler {
	inner := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			http.NotFound(w, req)
			return
		}
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			http.NotFound(w, req)
			return
		}
		if strings.Contains(path, "..") {
			http.NotFound(w, req)
			return
		}
		if !staticPathAllowed(scope, path) {
			http.NotFound(w, req)
			return
		}
		applyStaticCacheHeaders(w, path)
		inner.ServeHTTP(w, req)
	})
}

// applyStaticCacheHeaders 根据扩展名写入缓存控制头。保证前端发布后用户强刷即可获得新版。
func applyStaticCacheHeaders(w http.ResponseWriter, cleanPath string) {
	switch {
	case strings.HasSuffix(cleanPath, ".html"), cleanPath == "":
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
	case strings.HasSuffix(cleanPath, ".js"),
		strings.HasSuffix(cleanPath, ".css"),
		strings.HasSuffix(cleanPath, ".webmanifest"):
		w.Header().Set("Cache-Control", "no-cache, max-age=0")
	case strings.HasSuffix(cleanPath, ".svg"),
		strings.HasSuffix(cleanPath, ".png"),
		strings.HasSuffix(cleanPath, ".ico"):
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
}

func staticPathAllowed(scope staticScope, cleanPath string) bool {
	switch scope {
	case staticScopeUser:
		blocked := map[string]struct{}{
			"admin.html":       {},
			"admin-login.html": {},
			"js/admin.js":      {},
		}
		_, bad := blocked[cleanPath]
		return !bad
	case staticScopeAdmin:
		blocked := map[string]struct{}{
			"app.html":            {},
			"login.html":          {},
			"register.html":       {},
			"reset-password.html": {},
			"js/app.js":           {},
		}
		_, bad := blocked[cleanPath]
		return !bad
	default:
		return false
	}
}
