// Package api 控制面路由组装：用户平面与管理平面分端口；共享认证与业务依赖。
// @ai_doc_flow 用户平面: {control_path_prefix}/auth/* + /account/* + /me/* + 用户控制台静态资源
// @ai_doc_flow 管理平面: {control_path_prefix}/auth/* + /account/* + /admin/* + 管理后台静态资源
package api

import (
	"database/sql"
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/pkg/httputil"
)

//go:embed all:static
var staticFS embed.FS

// NewUserRouter 用户控制台平面（注册、登录、控制台；不包含 /admin API）。
func NewUserRouter(cfg *config.Config, db *sql.DB, rdb *redis.Client) http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(httputil.RequestIDMiddleware())
	r.Use(corsMiddleware())

	prefix := cfg.Server.NormalizedControlPathPrefix()
	p := newControlPlane(cfg, db, rdb)
	mountControlRoutes(r, cfg, db, rdb, prefix, p, mountOptions{
		scope:        staticScopeUser,
		indexTarget:  "/app.html",
		registerAPI:  p.registerUserAPI,
	})
	mountRootRedirect(r, prefix, "/app.html")

	return r
}

// NewUnifiedControlRouter 用户控制台 + 管理后台合并单端口（control_port == admin_control_port 时使用）。
func NewUnifiedControlRouter(cfg *config.Config, db *sql.DB, rdb *redis.Client) http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(httputil.RequestIDMiddleware())
	r.Use(corsMiddleware())

	prefix := cfg.Server.NormalizedControlPathPrefix()
	p := newControlPlane(cfg, db, rdb)
	mountControlRoutes(r, cfg, db, rdb, prefix, p, mountOptions{
		scope:       staticScopeUnified,
		indexTarget: "/login.html",
		registerAPI: func(g *gin.RouterGroup) {
			p.registerUserAPI(g)
			p.registerAdminAPI(g)
		},
	})
	mountRootRedirect(r, prefix, "/login.html")

	return r
}

// NewAdminRouter 管理后台平面（管理员 API + 管理端静态资源；不包含 /me）。
func NewAdminRouter(cfg *config.Config, db *sql.DB, rdb *redis.Client) http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(httputil.RequestIDMiddleware())
	r.Use(corsMiddleware())

	prefix := cfg.Server.NormalizedControlPathPrefix()
	p := newControlPlane(cfg, db, rdb)
	mountControlRoutes(r, cfg, db, rdb, prefix, p, mountOptions{
		scope:       staticScopeAdmin,
		indexTarget: "/admin-login.html",
		registerAPI: p.registerAdminAPI,
	})
	mountRootRedirect(r, prefix, "/admin-login.html")

	return r
}

type mountOptions struct {
	scope       staticScope
	indexTarget string
	registerAPI func(*gin.RouterGroup)
}

func mountControlRoutes(r *gin.Engine, cfg *config.Config, db *sql.DB, rdb *redis.Client, prefix string, p *controlPlane, opts mountOptions) {
	g := r.Group(prefix)

	g.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
	g.GET("/readyz", readyzHandler(db, rdb))
	g.GET("/traffic-config.js", trafficConfigHandler(cfg))

	opts.registerAPI(g)

	sub, _ := fs.Sub(staticFS, "static")
	staticH := newScopedStaticHandler(sub, opts.scope)

	g.GET("/", redirectWithPrefix(prefix, opts.indexTarget))
	g.GET("/docs", redirectWithPrefix(prefix, "/docs.html"))
	g.GET("/*filepath", staticCatchAll(staticH))
}

func staticCatchAll(staticH http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		fp := strings.TrimPrefix(c.Param("filepath"), "/")
		if fp == "" {
			c.Status(http.StatusNotFound)
			return
		}
		if strings.Contains(fp, "..") {
			c.Status(http.StatusNotFound)
			return
		}
		req := c.Request.Clone(c.Request.Context())
		req.URL.Path = "/" + fp
		staticH.ServeHTTP(c.Writer, req)
	}
}

func readyzHandler(db *sql.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := db.Ping(); err != nil {
			c.JSON(503, gin.H{"status": "mysql not ready", "error": err.Error()})
			return
		}
		if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(503, gin.H{"status": "redis not ready", "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Request-Id")
		c.Header("Access-Control-Expose-Headers", "X-Request-Id")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
