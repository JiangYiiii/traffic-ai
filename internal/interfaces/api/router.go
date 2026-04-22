// Package api 控制面路由组装：用户平面与管理平面分端口；共享认证与业务依赖。
// @ai_doc_flow 用户平面: /auth/* + /account/* + /me/* + 用户控制台静态资源
// @ai_doc_flow 管理平面: /auth/* + /account/* + /admin/* + 管理后台静态资源
package api

import (
	"database/sql"
	"embed"
	"io/fs"
	"net/http"

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

	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
	r.GET("/readyz", readyzHandler(db, rdb))

	p := newControlPlane(cfg, db, rdb)
	p.registerUserAPI(r)

	sub, _ := fs.Sub(staticFS, "static")
	staticH := newScopedStaticHandler(sub, staticScopeUser)
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/app.html") })
	r.GET("/docs", func(c *gin.Context) { c.Redirect(http.StatusFound, "/docs.html") })
	r.NoRoute(gin.WrapH(staticH))

	return r
}

// NewAdminRouter 管理后台平面（管理员 API + 管理端静态资源；不包含 /me）。
func NewAdminRouter(cfg *config.Config, db *sql.DB, rdb *redis.Client) http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(httputil.RequestIDMiddleware())
	r.Use(corsMiddleware())

	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
	r.GET("/readyz", readyzHandler(db, rdb))

	p := newControlPlane(cfg, db, rdb)
	p.registerAdminAPI(r)

	sub, _ := fs.Sub(staticFS, "static")
	staticH := newScopedStaticHandler(sub, staticScopeAdmin)
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/admin-login.html") })
	r.NoRoute(gin.WrapH(staticH))

	return r
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
