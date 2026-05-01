// Package gateway 数据面路由组装：多协议转发入口。
// @ai_doc_flow 数据面路由: /v1/chat/completions + /v1/images/generations + /v1/images/edits + /v1/files* + /v1/models + /v1/embeddings + /v1/messages 等
package gateway

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	billinguc "github.com/trailyai/traffic-ai/internal/application/billing"
	gwuc "github.com/trailyai/traffic-ai/internal/application/gateway"
	rluc "github.com/trailyai/traffic-ai/internal/application/ratelimit"
	routinguc "github.com/trailyai/traffic-ai/internal/application/routing"
	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/internal/infrastructure/httpclient"
	mysqlrepo "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	redisinfra "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
	"github.com/trailyai/traffic-ai/pkg/httputil"
)

func NewRouter(cfg *config.Config, db *sql.DB, rdb *redis.Client, metrics *Metrics, httpMgr *httpclient.Manager, breaker domainRouting.CircuitBreaker, maxAttempts int) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(httputil.RequestIDMiddleware())
	r.Use(CORSMiddleware())

	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
	r.GET("/metrics", gin.WrapH(metrics.Handler()))
	r.GET("/readyz", readyzHandler(db, rdb))

	// ---- 组装依赖 ----
	tokenRepo := mysqlrepo.NewTokenRepo(db)
	modelRepo := mysqlrepo.NewModelRepo(db)
	modelAccountRepo := mysqlrepo.NewModelAccountRepo(db)
	tgRepo := mysqlrepo.NewTokenGroupRepo(db)
	rlRuleRepo := mysqlrepo.NewRateLimitRuleRepo(db)
	balanceRepo := mysqlrepo.NewBalanceRepo(db)
	balanceLogRepo := mysqlrepo.NewBalanceLogRepo(db)
	redeemRepo := mysqlrepo.NewRedeemCodeRepo(db)
	usageLogRepo := mysqlrepo.NewUsageLogRepo(db)

	balanceCache := redisinfra.NewBalanceCache(rdb)
	monitorCounter := redisinfra.NewMonitorCounter(rdb)

	aesKey := []byte(cfg.Crypto.AESKey)

	// 限流
	rlUC := rluc.NewUseCase(rlRuleRepo, redisinfra.NewRedisRateLimiter(rdb, nil))
	// 用 ruleFunc 闭包注入活跃规则
	rateLimiter := redisinfra.NewRedisRateLimiter(rdb, rlUC.ActiveRules)

	// 路由引擎
	routingSvc := routinguc.NewUseCase(tgRepo, modelRepo, modelAccountRepo, aesKey, cfg.OAuth, breaker)

	// 计费
	billingSvc := billinguc.NewUseCase(db, balanceRepo, balanceLogRepo, redeemRepo, balanceCache)

	// 网关核心 UseCase
	uc := gwuc.NewUseCase(tokenRepo, routingSvc, billingSvc, rateLimiter, usageLogRepo, monitorCounter, httpMgr, breaker, maxAttempts, metrics)

	// ---- 路由注册 ----
	h := NewHandler(uc)

	authMW := AuthMiddleware(uc)

	v1 := r.Group("/v1")
	v1.Use(authMW)
	{
		v1.GET("/models", h.ListModels)
		v1.POST("/chat/completions", h.ChatCompletions)
		v1.POST("/images/generations", h.ImagesGenerations)
		v1.POST("/images/edits", h.ImagesEdits)
		v1.GET("/files", h.FilesList)
		v1.POST("/files", h.FilesCreate)
		v1.GET("/files/:file_id/content", h.FilesDownloadContent)
		v1.GET("/files/:file_id", h.FilesRetrieve)
		v1.DELETE("/files/:file_id", h.FilesDelete)
		v1.POST("/embeddings", h.Embeddings)
		v1.POST("/responses", h.Responses)
		v1.POST("/audio/speech", h.AudioSpeech)
		v1.POST("/messages", h.AnthropicMessages)
	}

	v1beta := r.Group("/v1beta")
	v1beta.Use(authMW)
	{
		v1beta.POST("/models/*modelAction", h.GeminiGenerateContent)
	}

	return r
}

// readyzHandler 深度健康检查：并行 ping DB 和 Redis，30ms 硬超时。
// 任一失败返回 503，但会**继续检查另一个**以便运维一眼看出两边状态。
func readyzHandler(db *sql.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Millisecond)
		defer cancel()

		result := gin.H{"db": "ok", "redis": "ok"}
		httpStatus := http.StatusOK

		if err := db.PingContext(ctx); err != nil {
			result["db"] = "fail: " + err.Error()
			httpStatus = http.StatusServiceUnavailable
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			result["redis"] = "fail: " + err.Error()
			httpStatus = http.StatusServiceUnavailable
		}
		c.JSON(httpStatus, result)
	}
}
