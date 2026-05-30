// @ai_doc_flow 控制面入口: 加载配置 → 初始化 DB/Redis → 用户平面 + 管理平面 HTTP 服务
// 控制面承载: 用户认证、API Key 管理、模型配置、计费、审计、控制台前端
// 当 control_port == admin_control_port 时合并为单端口；否则双端口分离
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	mysqlpkg "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	redispkg "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
	"github.com/trailyai/traffic-ai/internal/interfaces/api"
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
	if redispkg.SkipCheck() {
		logger.L.Warnw("TRAFFIC_REDIS_SKIP_CHECK=1: redis startup ping and /readyz redis check disabled; use GET /debug/redis for diagnostics")
	}

	servers := buildControlServers(cfg, db, rdb)
	for _, srv := range servers {
		go func(s *http.Server) {
			logger.L.Infof("control plane listening on %s", s.Addr)
			if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.L.Fatalf("control plane listen: %v", err)
			}
		}(srv)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.L.Info("shutting down control planes...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(ctx)
	}
}

func buildControlServers(cfg *config.Config, db *sql.DB, rdb *redis.Client) []*http.Server {
	if cfg.Server.UnifiedControlPort() {
		port := cfg.Server.ControlPort
		logger.L.Infof("unified control plane (user + admin) on :%d", port)
		return []*http.Server{{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: api.NewUnifiedControlRouter(cfg, db, rdb),
		}}
	}

	logger.L.Infof("split control planes: user :%d, admin :%d", cfg.Server.ControlPort, cfg.Server.AdminControlPort)
	return []*http.Server{
		{
			Addr:    fmt.Sprintf(":%d", cfg.Server.ControlPort),
			Handler: api.NewUserRouter(cfg, db, rdb),
		},
		{
			Addr:    fmt.Sprintf(":%d", cfg.Server.AdminControlPort),
			Handler: api.NewAdminRouter(cfg, db, rdb),
		},
	}
}
