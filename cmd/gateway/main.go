// @ai_doc_flow 数据面入口: 加载配置 → 初始化 DB/Redis → 启动网关 HTTP 服务
// 数据面承载: API Key 鉴权、限流、路由选择、上游转发、流式 SSE、降级、计费
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/internal/infrastructure/httpclient"
	mysqlpkg "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	redispkg "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
	"github.com/trailyai/traffic-ai/internal/interfaces/gateway"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output, cfg.Log.FilePath)
	defer logger.Sync()

	db, err := mysqlpkg.NewDB(&cfg.Database)
	if err != nil {
		logger.L.Fatalf("init mysql: %v", err)
	}
	defer db.Close()

	rdb, err := redispkg.NewClient(&cfg.Redis)
	if err != nil {
		logger.L.Fatalf("init redis: %v", err)
	}
	defer rdb.Close()

	metrics := gateway.NewMetrics()

	// 上游 HTTP 客户端池化：按账号缓存独立 Transport，支持分项超时。
	// enabled=false 时传入 nil，由 UseCase.getUpstreamClient 自动回退到裸 http.Client 路径。
	// TRAFFIC_UPSTREAM_LEGACY=1 是运行时紧急回退开关，由 getUpstreamClient 读环境变量判断，
	// 此处仅打印告警日志便于运维一眼看出降级状态。
	var httpMgr *httpclient.Manager
	if cfg.Gateway.Upstream.Enabled {
		httpMgr = httpclient.NewManager(httpclient.Config{
			MaxIdleConns:          cfg.Gateway.Upstream.MaxIdleConns,
			MaxIdleConnsPerHost:   cfg.Gateway.Upstream.MaxIdleConnsPerHost,
			MaxConnsPerHost:       cfg.Gateway.Upstream.MaxConnsPerHost,
			IdleConnTimeout:       time.Duration(cfg.Gateway.Upstream.IdleConnTimeoutSec) * time.Second,
			DialTimeout:           time.Duration(cfg.Gateway.Upstream.DialTimeoutSec) * time.Second,
			TLSHandshakeTimeout:   time.Duration(cfg.Gateway.Upstream.TLSHandshakeTimeoutSec) * time.Second,
			ResponseHeaderTimeout: time.Duration(cfg.Gateway.Upstream.ResponseHeaderTimeoutSec) * time.Second,
			StreamIdleTimeout:     time.Duration(cfg.Gateway.Upstream.StreamIdleTimeoutSec) * time.Second,
		})
		defer httpMgr.Close()
	}
	if !cfg.Gateway.Upstream.Enabled || os.Getenv("TRAFFIC_UPSTREAM_LEGACY") == "1" {
		logger.L.Warn("upstream pool disabled, using legacy http.Client (TRAFFIC_UPSTREAM_LEGACY=1 or gateway.upstream.enabled=false)")
	}

	// 账号级熔断：cfg.Gateway.Circuit.Enabled=false 时传 nil，
	// routing 层会自动降级为"不过滤"；同时 maxAttempts 也强制为 1，fallback 循环退化为单次请求。
	var breaker domainRouting.CircuitBreaker
	maxAttempts := cfg.Gateway.Circuit.MaxAttempts
	if cfg.Gateway.Circuit.Enabled {
		breaker = redispkg.NewRedisCircuitBreaker(rdb, redispkg.BreakerConfig{
			ErrorRateThreshold:      cfg.Gateway.Circuit.ErrorRateThreshold,
			MinRequestCount:         cfg.Gateway.Circuit.MinRequestCount,
			WindowSec:               cfg.Gateway.Circuit.WindowSec,
			CooldownBaseMs:          cfg.Gateway.Circuit.CooldownBaseMs,
			CooldownMaxMs:           cfg.Gateway.Circuit.CooldownMaxMs,
			SuccessThresholdToClose: cfg.Gateway.Circuit.SuccessThresholdToClose,
			HalfOpenProbeRate:       cfg.Gateway.Circuit.HalfOpenProbeRate,
			KeyTTLSec:               cfg.Gateway.Circuit.KeyTTLSec,
		})
		if maxAttempts <= 0 {
			maxAttempts = 3
		}
	} else {
		logger.L.Warn("circuit breaker disabled by config, accounts will not be auto-isolated")
		maxAttempts = 1
	}

	router := gateway.NewRouter(cfg, db, rdb, metrics, httpMgr, breaker, maxAttempts)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.GatewayPort),
		Handler: router,
	}

	go func() {
		logger.L.Infof("gateway starting on :%d", cfg.Server.GatewayPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.L.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.L.Info("shutting down gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.L.Errorf("shutdown: %v", err)
	}
}
