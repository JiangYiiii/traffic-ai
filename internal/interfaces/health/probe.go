package health

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// HealthzHandler 轻量存活探针。
func HealthzHandler() gin.HandlerFunc {
	return func(c *gin.Context) { c.String(200, "ok") }
}

// MountProbes 注册健康检查。
// prefix 为空时仅在 group 注册 /healthz；有 prefix 时在 Engine 根路径额外注册，供 K8s 探针打 /healthz。
func MountProbes(r *gin.Engine, g *gin.RouterGroup, prefix string, db *sql.DB, rdb *redis.Client) {
	healthz := HealthzHandler()
	readyz := ReadyzHandler(db, rdb)

	g.GET("/healthz", healthz)
	g.GET("/readyz", readyz)

	if prefix != "" {
		r.GET("/healthz", healthz)
		r.GET("/readyz", readyz)
	}
}
