package redis

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
)

type probeResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DiagnosticReport 返回 Redis 连通性诊断信息，供 /debug/redis 使用。
func DiagnosticReport(ctx context.Context, cfg *config.RedisConfig, rdb *redis.Client) map[string]any {
	report := map[string]any{
		"configured_addr":   cfg.Addr,
		"db":                cfg.DB,
		"password_set":      cfg.Password != "",
		"skip_check_enabled": SkipCheck(),
		"tcp":               probeTCP(ctx, cfg.Addr),
	}

	if rdb != nil {
		start := time.Now()
		err := rdb.Ping(ctx).Err()
		res := probeResult{LatencyMs: time.Since(start).Milliseconds()}
		if err != nil {
			res.Error = err.Error()
		} else {
			res.OK = true
		}
		report["redis_ping"] = res
	} else {
		report["redis_ping"] = probeResult{Error: "redis client is nil"}
	}

	return report
}

func probeTCP(ctx context.Context, addr string) probeResult {
	start := time.Now()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	res := probeResult{LatencyMs: time.Since(start).Milliseconds()}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	_ = conn.Close()
	res.OK = true
	return res
}

// PingOrSkip 用于 /readyz：skip 模式返回 skipped 状态，否则执行 ping。
func PingOrSkip(ctx context.Context, rdb *redis.Client) (status string, err error) {
	if SkipCheck() {
		return fmt.Sprintf("skipped (TRAFFIC_REDIS_SKIP_CHECK=%s)", envSkipValue()), nil
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		return "fail", err
	}
	return "ok", nil
}

func envSkipValue() string {
	v := os.Getenv("TRAFFIC_REDIS_SKIP_CHECK")
	if v == "" {
		return "1"
	}
	return v
}
