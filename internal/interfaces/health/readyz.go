package health

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	redispkg "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
)

// RegisterDebugRoutes 在 skip-check 模式下暴露 Redis 诊断端点。
func RegisterDebugRoutes(g *gin.RouterGroup, cfg *config.RedisConfig, rdb *redis.Client) {
	if !redispkg.SkipCheck() {
		return
	}
	g.GET("/debug/redis", redisDebugHandler(cfg, rdb))
}

func redisDebugHandler(cfg *config.RedisConfig, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		c.JSON(http.StatusOK, redispkg.DiagnosticReport(ctx, cfg, rdb))
	}
}

// ReadyzHandler 就绪检查：MySQL 必检；Redis 在 TRAFFIC_REDIS_SKIP_CHECK=1 时跳过。
func ReadyzHandler(db *sql.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		result := gin.H{}
		httpStatus := http.StatusOK

		if err := db.PingContext(ctx); err != nil {
			result["db"] = "fail: " + err.Error()
			httpStatus = http.StatusServiceUnavailable
		} else {
			result["db"] = "ok"
		}

		redisStatus, err := redispkg.PingOrSkip(ctx, rdb)
		if err != nil {
			result["redis"] = "fail: " + err.Error()
			httpStatus = http.StatusServiceUnavailable
		} else {
			result["redis"] = redisStatus
		}

		if redispkg.SkipCheck() {
			result["redis_skip_check"] = true
		}

		if httpStatus == http.StatusOK {
			c.JSON(httpStatus, gin.H{"status": "ready", "checks": result})
			return
		}
		c.JSON(httpStatus, gin.H{"status": "not ready", "checks": result})
	}
}
