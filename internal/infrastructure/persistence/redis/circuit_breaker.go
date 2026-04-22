package redis

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// BreakerConfig 熔断器运行参数。所有 0 值字段会在构造时被替换为默认值。
type BreakerConfig struct {
	ErrorRateThreshold      float64       // 默认 0.5
	MinRequestCount         int           // 默认 20
	WindowSec               int           // 默认 60
	CooldownBaseMs          int           // 默认 5000
	CooldownMaxMs           int           // 默认 60000
	SuccessThresholdToClose int           // 默认 3
	HalfOpenProbeRate       float64       // 默认 0.3
	KeyPrefix               string        // 默认 "traffic:circuit:account:"
	KeyTTLSec               int           // 默认 600
}

// RedisCircuitBreaker 基于 Redis Hash 的账号级熔断器。
//
// 关键决策：HGETALL + HSET 之间非原子。race 的后果是偶发多放 1~2 个请求进 half_open、
// 或 failure 多计几次。这是可接受的最终一致，换来的是不依赖 Lua 脚本、运维可直接用 redis-cli 读写。
// 若后续要强一致，需要整体替换为 Lua 脚本（本卡不做）。
type RedisCircuitBreaker struct {
	rdb *redis.Client
	cfg BreakerConfig
}

// NewRedisCircuitBreaker 构造实例。cfg 中未显式设置的字段使用默认值。
func NewRedisCircuitBreaker(rdb *redis.Client, cfg BreakerConfig) *RedisCircuitBreaker {
	applyBreakerDefaults(&cfg)
	return &RedisCircuitBreaker{rdb: rdb, cfg: cfg}
}

func applyBreakerDefaults(c *BreakerConfig) {
	if c.ErrorRateThreshold <= 0 {
		c.ErrorRateThreshold = 0.5
	}
	if c.MinRequestCount <= 0 {
		c.MinRequestCount = 20
	}
	if c.WindowSec <= 0 {
		c.WindowSec = 60
	}
	if c.CooldownBaseMs <= 0 {
		c.CooldownBaseMs = 5000
	}
	if c.CooldownMaxMs <= 0 {
		c.CooldownMaxMs = 60000
	}
	if c.SuccessThresholdToClose <= 0 {
		c.SuccessThresholdToClose = 3
	}
	if c.HalfOpenProbeRate <= 0 {
		c.HalfOpenProbeRate = 0.3
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = "traffic:circuit:account:"
	}
	if c.KeyTTLSec <= 0 {
		c.KeyTTLSec = 600
	}
}

func (b *RedisCircuitBreaker) key(accountID int64) string {
	return b.cfg.KeyPrefix + strconv.FormatInt(accountID, 10)
}

const (
	stateClosed   = "closed"
	stateOpen     = "open"
	stateHalfOpen = "half_open"

	fieldState        = "state"
	fieldWinStart     = "win_start_ms"
	fieldFailures     = "failures"
	fieldSamples      = "samples"
	fieldOpenedAt     = "opened_at_ms"
	fieldCooldown     = "cooldown_ms"
	fieldHalfOpenOK   = "half_open_ok"
)

func nowMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func parseHashInt64(m map[string]string, k string) int64 {
	if v, ok := m[k]; ok && v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

// Allow 判断是否允许调用。
//
// closed：直接放行。
// open：如果 now >= opened_at_ms + cooldown_ms，就迁移到 half_open 并放行本请求（作为第一个探测），
//
//	否则拒绝。
// half_open：按 HalfOpenProbeRate 概率放行。
func (b *RedisCircuitBreaker) Allow(ctx context.Context, accountID int64) (bool, error) {
	key := b.key(accountID)
	h, err := b.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return true, fmt.Errorf("breaker hgetall: %w", err)
	}
	state := h[fieldState]
	if state == "" {
		state = stateClosed
	}
	now := nowMs()

	switch state {
	case stateClosed:
		return true, nil

	case stateOpen:
		openedAt := parseHashInt64(h, fieldOpenedAt)
		cooldown := parseHashInt64(h, fieldCooldown)
		if cooldown <= 0 {
			cooldown = int64(b.cfg.CooldownBaseMs)
		}
		if now >= openedAt+cooldown {
			pipe := b.rdb.Pipeline()
			pipe.HSet(ctx, key, fieldState, stateHalfOpen, fieldHalfOpenOK, 0)
			pipe.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
			if _, err := pipe.Exec(ctx); err != nil {
				return true, fmt.Errorf("breaker open→half_open: %w", err)
			}
			return true, nil
		}
		return false, nil

	case stateHalfOpen:
		return rand.Float64() < b.cfg.HalfOpenProbeRate, nil

	default:
		return true, nil
	}
}

// RecordSuccess 见文件头算法描述。
func (b *RedisCircuitBreaker) RecordSuccess(ctx context.Context, accountID int64) error {
	key := b.key(accountID)
	h, err := b.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("breaker hgetall: %w", err)
	}
	state := h[fieldState]
	if state == "" {
		state = stateClosed
	}
	now := nowMs()

	if state == stateHalfOpen {
		n, err := b.rdb.HIncrBy(ctx, key, fieldHalfOpenOK, 1).Result()
		if err != nil {
			return fmt.Errorf("breaker half_open incr: %w", err)
		}
		b.touch(ctx, key)
		if int(n) >= b.cfg.SuccessThresholdToClose {
			pipe := b.rdb.Pipeline()
			pipe.HSet(ctx, key,
				fieldState, stateClosed,
				fieldFailures, 0,
				fieldSamples, 0,
				fieldWinStart, now,
				fieldCooldown, b.cfg.CooldownBaseMs,
				fieldHalfOpenOK, 0,
			)
			pipe.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
			if _, err := pipe.Exec(ctx); err != nil {
				return fmt.Errorf("breaker half_open→closed: %w", err)
			}
		}
		return nil
	}

	// open 状态下 Allow 已拒绝；若仍收到 Success，忽略即可。
	if state == stateOpen {
		return nil
	}

	// closed：滚动窗口统计样本。
	winStart := parseHashInt64(h, fieldWinStart)
	if winStart == 0 || now-winStart > int64(b.cfg.WindowSec)*1000 {
		pipe := b.rdb.Pipeline()
		pipe.HSet(ctx, key, fieldFailures, 0, fieldSamples, 1, fieldWinStart, now)
		pipe.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("breaker reset window success: %w", err)
		}
		return nil
	}
	if _, err := b.rdb.HIncrBy(ctx, key, fieldSamples, 1).Result(); err != nil {
		return fmt.Errorf("breaker incr samples: %w", err)
	}
	b.touch(ctx, key)
	return nil
}

// RecordFailure 见文件头算法描述。
func (b *RedisCircuitBreaker) RecordFailure(ctx context.Context, accountID int64, _ string) error {
	key := b.key(accountID)
	h, err := b.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("breaker hgetall: %w", err)
	}
	state := h[fieldState]
	if state == "" {
		state = stateClosed
	}
	now := nowMs()

	if state == stateHalfOpen {
		prev := parseHashInt64(h, fieldCooldown)
		if prev < int64(b.cfg.CooldownBaseMs) {
			prev = int64(b.cfg.CooldownBaseMs)
		}
		newCooldown := prev * 2
		if newCooldown > int64(b.cfg.CooldownMaxMs) {
			newCooldown = int64(b.cfg.CooldownMaxMs)
		}
		pipe := b.rdb.Pipeline()
		pipe.HSet(ctx, key,
			fieldState, stateOpen,
			fieldOpenedAt, now,
			fieldCooldown, newCooldown,
			fieldHalfOpenOK, 0,
		)
		pipe.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("breaker half_open→open: %w", err)
		}
		return nil
	}

	if state == stateOpen {
		// 已在 open，不重复计数；续期避免 key 过早过期。
		b.touch(ctx, key)
		return nil
	}

	// closed：滚动窗口并判断是否触发 open。
	winStart := parseHashInt64(h, fieldWinStart)
	if winStart == 0 || now-winStart > int64(b.cfg.WindowSec)*1000 {
		pipe := b.rdb.Pipeline()
		pipe.HSet(ctx, key, fieldFailures, 1, fieldSamples, 1, fieldWinStart, now)
		pipe.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("breaker reset window failure: %w", err)
		}
	} else {
		pipe := b.rdb.Pipeline()
		pipe.HIncrBy(ctx, key, fieldFailures, 1)
		pipe.HIncrBy(ctx, key, fieldSamples, 1)
		pipe.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("breaker incr failure: %w", err)
		}
	}

	// 重读判断是否触发熔断。
	h2, err := b.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("breaker reread: %w", err)
	}
	samples := parseHashInt64(h2, fieldSamples)
	failures := parseHashInt64(h2, fieldFailures)
	if samples >= int64(b.cfg.MinRequestCount) &&
		float64(failures)/float64(samples) >= b.cfg.ErrorRateThreshold {
		pipe := b.rdb.Pipeline()
		pipe.HSet(ctx, key,
			fieldState, stateOpen,
			fieldOpenedAt, now,
			fieldCooldown, b.cfg.CooldownBaseMs,
			fieldHalfOpenOK, 0,
		)
		pipe.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("breaker closed→open: %w", err)
		}
	}
	return nil
}

// State 返回当前状态。未知或空键返回 "closed"。
func (b *RedisCircuitBreaker) State(ctx context.Context, accountID int64) (string, error) {
	v, err := b.rdb.HGet(ctx, b.key(accountID), fieldState).Result()
	if err == redis.Nil {
		return stateClosed, nil
	}
	if err != nil {
		return stateClosed, fmt.Errorf("breaker hget state: %w", err)
	}
	switch v {
	case stateClosed, stateOpen, stateHalfOpen:
		return v, nil
	default:
		return stateClosed, nil
	}
}

// touch 续期 key，忽略错误（key 可能已经被别的操作续过期）。
func (b *RedisCircuitBreaker) touch(ctx context.Context, key string) {
	b.rdb.Expire(ctx, key, time.Duration(b.cfg.KeyTTLSec)*time.Second)
}
