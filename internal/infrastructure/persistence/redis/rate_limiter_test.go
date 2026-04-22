package redis

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	domain "github.com/trailyai/traffic-ai/internal/domain/ratelimit"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

func TestMain(m *testing.M) {
	logger.Init("error", "text", "stdout", "")
	os.Exit(m.Run())
}

func newTestRateLimiter(t *testing.T, rules []*domain.RateLimitRule) (*RedisRateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewRedisRateLimiter(rdb, func() []*domain.RateLimitRule { return rules }), mr
}

func TestTPM_NotConfigured_Skip(t *testing.T) {
	rules := []*domain.RateLimitRule{{
		ID:     1,
		Scope:  domain.ScopeUser,
		Name:   "u1",
		MaxRPM: 100, MaxTPM: 0, // 未配置 TPM
		ScopeValue: "42", IsActive: true,
	}}
	l, _ := newTestRateLimiter(t, rules)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := l.Allow(ctx, &domain.CheckRequest{UserID: 42, EstimatedTokens: 100000}); err != nil {
			t.Fatalf("unexpected block when MaxTPM=0: %v", err)
		}
	}
}

func TestTPM_ZeroEstimated_Skip(t *testing.T) {
	rules := []*domain.RateLimitRule{{
		ID: 2, Scope: domain.ScopeUser, Name: "u1",
		MaxTPM: 1000, ScopeValue: "42", IsActive: true,
	}}
	l, mr := newTestRateLimiter(t, rules)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := l.Allow(ctx, &domain.CheckRequest{UserID: 42, EstimatedTokens: 0}); err != nil {
			t.Fatalf("should skip TPM when estimated=0: %v", err)
		}
	}
	keys := mr.Keys()
	for _, k := range keys {
		if len(k) >= 6 && k[:6] == "rl:tpm" {
			t.Fatalf("expected no TPM key written, got %q", k)
		}
	}
}

func TestTPM_UnderLimit_Allow(t *testing.T) {
	rules := []*domain.RateLimitRule{{
		ID: 3, Scope: domain.ScopeUser, Name: "u1",
		MaxTPM: 10000, ScopeValue: "42", IsActive: true,
	}}
	l, _ := newTestRateLimiter(t, rules)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := l.Allow(ctx, &domain.CheckRequest{UserID: 42, EstimatedTokens: 3000}); err != nil {
			t.Fatalf("iter %d: expected allow, got: %v", i, err)
		}
	}
}

func TestTPM_OverLimit_Blocked(t *testing.T) {
	rules := []*domain.RateLimitRule{{
		ID: 4, Scope: domain.ScopeUser, Name: "u1",
		MaxTPM: 5000, ScopeValue: "42", IsActive: true,
	}}
	l, _ := newTestRateLimiter(t, rules)
	ctx := context.Background()
	if err := l.Allow(ctx, &domain.CheckRequest{UserID: 42, EstimatedTokens: 3000}); err != nil {
		t.Fatalf("first call should pass: %v", err)
	}
	err := l.Allow(ctx, &domain.CheckRequest{UserID: 42, EstimatedTokens: 3000})
	if !errors.Is(err, errcode.ErrRateLimited) {
		t.Fatalf("second call should be rate limited, got: %v", err)
	}
}

func TestTPM_RedisDown_Degrade(t *testing.T) {
	rules := []*domain.RateLimitRule{{
		ID: 5, Scope: domain.ScopeUser, Name: "u1",
		MaxTPM: 1000, ScopeValue: "42", IsActive: true,
	}}
	l, mr := newTestRateLimiter(t, rules)
	mr.Close() // 模拟 Redis 挂掉
	ctx := context.Background()
	if err := l.Allow(ctx, &domain.CheckRequest{UserID: 42, EstimatedTokens: 5000}); err != nil {
		t.Fatalf("expected degrade to allow on redis failure, got: %v", err)
	}
}

func TestTPM_GlobalScope(t *testing.T) {
	rules := []*domain.RateLimitRule{{
		ID: 6, Scope: domain.ScopeGlobal, Name: "g",
		MaxTPM: 5000, ScopeValue: "", IsActive: true,
	}}
	l, _ := newTestRateLimiter(t, rules)
	ctx := context.Background()
	if err := l.Allow(ctx, &domain.CheckRequest{EstimatedTokens: 3000}); err != nil {
		t.Fatalf("global first call should pass: %v", err)
	}
	err := l.Allow(ctx, &domain.CheckRequest{EstimatedTokens: 3000})
	if !errors.Is(err, errcode.ErrRateLimited) {
		t.Fatalf("global second call should be rate limited, got: %v", err)
	}
}

func TestTPM_KeyFormat(t *testing.T) {
	got := tpmKey(domain.ScopeUser, "42", 12345)
	want := "rl:tpm:user:42:12345"
	if got != want {
		t.Fatalf("tpmKey mismatch: got=%q want=%q", got, want)
	}
	got2 := tpmKey(domain.ScopeGlobal, "global", 0)
	if got2 != "rl:tpm:global:global:0" {
		t.Fatalf("tpmKey global mismatch: got=%q", got2)
	}
}
