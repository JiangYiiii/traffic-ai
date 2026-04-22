package redis

import (
	"context"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestBreaker(t *testing.T, cfg BreakerConfig) (*RedisCircuitBreaker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewRedisCircuitBreaker(rdb, cfg), mr
}

func TestBreaker_ClosedAllowsByDefault(t *testing.T) {
	b, _ := newTestBreaker(t, BreakerConfig{})
	ok, err := b.Allow(context.Background(), 1)
	if err != nil {
		t.Fatalf("Allow err: %v", err)
	}
	if !ok {
		t.Fatalf("expected allow=true on fresh account")
	}
	s, _ := b.State(context.Background(), 1)
	if s != stateClosed {
		t.Fatalf("expected state=closed, got %q", s)
	}
}

func TestBreaker_RecordFailureTrips(t *testing.T) {
	b, _ := newTestBreaker(t, BreakerConfig{
		ErrorRateThreshold: 0.5,
		MinRequestCount:    20,
		WindowSec:          60,
		CooldownBaseMs:     5000,
	})
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if err := b.RecordFailure(ctx, 42, "upstream_5xx"); err != nil {
			t.Fatalf("RecordFailure err: %v", err)
		}
	}
	ok, err := b.Allow(ctx, 42)
	if err != nil {
		t.Fatalf("Allow err: %v", err)
	}
	if ok {
		t.Fatalf("expected allow=false after tripping breaker")
	}
	s, _ := b.State(ctx, 42)
	if s != stateOpen {
		t.Fatalf("expected state=open, got %q", s)
	}
}

func TestBreaker_OpenAllowsAfterCooldown(t *testing.T) {
	b, mr := newTestBreaker(t, BreakerConfig{
		CooldownBaseMs:    5000,
		HalfOpenProbeRate: 1.0,
	})
	ctx := context.Background()
	key := b.key(7)

	openedAt := nowMs() - 6000
	mr.HSet(key, fieldState, stateOpen)
	mr.HSet(key, fieldOpenedAt, strconv.FormatInt(openedAt, 10))
	mr.HSet(key, fieldCooldown, "5000")

	ok, err := b.Allow(ctx, 7)
	if err != nil {
		t.Fatalf("Allow err: %v", err)
	}
	if !ok {
		t.Fatalf("expected allow=true after cooldown elapsed")
	}
	s, _ := b.State(ctx, 7)
	if s != stateHalfOpen {
		t.Fatalf("expected state=half_open after cooldown transition, got %q", s)
	}
}

func TestBreaker_HalfOpenCloseOnSuccesses(t *testing.T) {
	b, mr := newTestBreaker(t, BreakerConfig{
		SuccessThresholdToClose: 3,
		HalfOpenProbeRate:       1.0,
	})
	ctx := context.Background()
	key := b.key(101)
	mr.HSet(key, fieldState, stateHalfOpen)
	mr.HSet(key, fieldHalfOpenOK, "0")

	for i := 0; i < 3; i++ {
		if err := b.RecordSuccess(ctx, 101); err != nil {
			t.Fatalf("RecordSuccess err: %v", err)
		}
	}
	s, _ := b.State(ctx, 101)
	if s != stateClosed {
		t.Fatalf("expected state=closed after %d successes, got %q", 3, s)
	}
	ok, err := b.Allow(ctx, 101)
	if err != nil {
		t.Fatalf("Allow err: %v", err)
	}
	if !ok {
		t.Fatalf("expected allow=true after recovery")
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	b, mr := newTestBreaker(t, BreakerConfig{
		CooldownBaseMs: 5000,
		CooldownMaxMs:  60000,
	})
	ctx := context.Background()
	key := b.key(202)
	mr.HSet(key, fieldState, stateHalfOpen)
	mr.HSet(key, fieldCooldown, "5000")

	if err := b.RecordFailure(ctx, 202, "upstream_5xx"); err != nil {
		t.Fatalf("RecordFailure err: %v", err)
	}
	s, _ := b.State(ctx, 202)
	if s != stateOpen {
		t.Fatalf("expected state=open after half_open failure, got %q", s)
	}
	cooldownStr := mr.HGet(key, fieldCooldown)
	cd, _ := strconv.ParseInt(cooldownStr, 10, 64)
	if cd != 10000 {
		t.Fatalf("expected cooldown doubled to 10000, got %d", cd)
	}
}

func TestBreaker_State(t *testing.T) {
	b, mr := newTestBreaker(t, BreakerConfig{})
	ctx := context.Background()

	s, err := b.State(ctx, 1)
	if err != nil {
		t.Fatalf("State err: %v", err)
	}
	if s != stateClosed {
		t.Fatalf("expected closed for fresh key, got %q", s)
	}

	mr.HSet(b.key(2), fieldState, stateOpen)
	if s, _ := b.State(ctx, 2); s != stateOpen {
		t.Fatalf("expected open, got %q", s)
	}

	mr.HSet(b.key(3), fieldState, stateHalfOpen)
	if s, _ := b.State(ctx, 3); s != stateHalfOpen {
		t.Fatalf("expected half_open, got %q", s)
	}

	mr.HSet(b.key(4), fieldState, "weird")
	if s, _ := b.State(ctx, 4); s != stateClosed {
		t.Fatalf("expected closed for unknown value, got %q", s)
	}
}
