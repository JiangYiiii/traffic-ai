// @ai_doc_flow 控制面入口: 加载配置 → 初始化 DB/Redis → 用户平面 + 管理平面双 HTTP 服务
// 控制面承载: 用户认证、API Key 管理、模型配置、计费、审计、控制台前端（用户端口与管理端口分离）
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

	userRouter := api.NewUserRouter(cfg, db, rdb)
	adminRouter := api.NewAdminRouter(cfg, db, rdb)

	userSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.ControlPort),
		Handler: userRouter,
	}
	adminSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.AdminControlPort),
		Handler: adminRouter,
	}

	go func() {
		logger.L.Infof("user control plane (console) starting on :%d", cfg.Server.ControlPort)
		if err := userSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.L.Fatalf("user plane listen: %v", err)
		}
	}()

	go func() {
		logger.L.Infof("admin control plane starting on :%d", cfg.Server.AdminControlPort)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.L.Fatalf("admin plane listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.L.Info("shutting down control planes...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = userSrv.Shutdown(ctx)
	_ = adminSrv.Shutdown(ctx)
}
