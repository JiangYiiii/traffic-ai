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

// MountRootProbes 在 Engine 根路径注册 /healthz、/readyz，供 K8s 探针使用（与 path_prefix 无关）。
func MountRootProbes(r *gin.Engine, db *sql.DB, rdb *redis.Client) {
	r.GET("/healthz", HealthzHandler())
	r.GET("/readyz", ReadyzHandler(db, rdb))
}
