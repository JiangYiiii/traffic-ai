package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	domain "github.com/trailyai/traffic-ai/internal/domain/ratelimit"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

// RedisRateLimiter 基于 Redis 的多级限流器。
// RPM: sorted set 滑动窗口 (score=unix_ms, member=unique_id)
// 并发: INCR/DECR 原子计数器
type RedisRateLimiter struct {
	rdb      *redis.Client
	ruleFunc func() []*domain.RateLimitRule // 获取当前活跃规则
}

func NewRedisRateLimiter(rdb *redis.Client, ruleFunc func() []*domain.RateLimitRule) *RedisRateLimiter {
	return &RedisRateLimiter{rdb: rdb, ruleFunc: ruleFunc}
}

func (l *RedisRateLimiter) Allow(ctx context.Context, req *domain.CheckRequest) error {
	rules := l.ruleFunc()
	now := time.Now()

	for _, scope := range domain.CheckOrder {
		scopeVal := scopeValueFromReq(scope, req)
		if scopeVal == "" && scope != domain.ScopeGlobal {
			continue
		}

		matched := l.findRules(rules, scope, scopeVal)
		for _, rule := range matched {
			if rule.HasRPMLimit() {
				if err := l.checkRPM(ctx, rule, scope, scopeVal, now); err != nil {
					return err
				}
			}
			if rule.HasConcurrentLimit() {
				if err := l.acquireConcurrent(ctx, rule, scope, scopeVal); err != nil {
					return err
				}
			}
			if rule.HasTPMLimit() && req.EstimatedTokens > 0 {
				if err := l.checkTPM(ctx, rule, scope, scopeVal, req.EstimatedTokens, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (l *RedisRateLimiter) Release(ctx context.Context, req *domain.CheckRequest) {
	rules := l.ruleFunc()
	for _, scope := range domain.CheckOrder {
		scopeVal := scopeValueFromReq(scope, req)
		if scopeVal == "" && scope != domain.ScopeGlobal {
			continue
		}
		matched := l.findRules(rules, scope, scopeVal)
		for _, rule := range matched {
			if rule.HasConcurrentLimit() {
				key := concurrentKey(scope, scopeVal)
				if err := l.rdb.Decr(ctx, key).Err(); err != nil {
					logger.L.Warnw("release concurrent failed", "key", key, "error", err)
				}
			}
		}
	}
}

// checkRPM 滑动窗口算法:
// 1. ZREMRANGEBYSCORE 清理 60s 之前的过期成员
// 2. ZCARD 统计窗口内计数
// 3. 若未超限则 ZADD 当前请求
// 使用 Pipeline 减少 RTT。
func (l *RedisRateLimiter) checkRPM(ctx context.Context, rule *domain.RateLimitRule, scope domain.Scope, scopeVal string, now time.Time) error {
	key := rpmKey(scope, scopeVal)
	windowStart := now.Add(-60 * time.Second)

	pipe := l.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(windowStart.UnixMilli(), 10))
	countCmd := pipe.ZCard(ctx, key)
	_, err := pipe.Exec(ctx)
	if err != nil {
		logger.L.Errorw("rpm pipeline exec failed", "key", key, "error", err)
		return nil // 限流降级：Redis 异常时放行
	}

	count := countCmd.Val()
	if count >= int64(rule.MaxRPM) {
		return &domain.RateLimitError{Scope: scope, Reason: domain.ReasonRPM}
	}

	member := fmt.Sprintf("%d:%d", now.UnixNano(), count)
	score := float64(now.UnixMilli())
	pipe2 := l.rdb.Pipeline()
	pipe2.ZAdd(ctx, key, redis.Z{Score: score, Member: member})
	pipe2.Expire(ctx, key, 70*time.Second) // 略大于窗口防止残留
	if _, err := pipe2.Exec(ctx); err != nil {
		logger.L.Warnw("rpm zadd failed", "key", key, "error", err)
	}
	return nil
}

func (l *RedisRateLimiter) acquireConcurrent(ctx context.Context, rule *domain.RateLimitRule, scope domain.Scope, scopeVal string) error {
	key := concurrentKey(scope, scopeVal)
	val, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		logger.L.Errorw("acquire concurrent failed", "key", key, "error", err)
		return nil // 降级放行
	}

	// 给并发 key 设置兜底过期，防止 Release 未调用时永久泄漏
	l.rdb.Expire(ctx, key, 10*time.Minute)

	if val > int64(rule.MaxConcurrent) {
		l.rdb.Decr(ctx, key)
		return &domain.RateLimitError{Scope: scope, Reason: domain.ReasonConcurrent}
	}
	return nil
}

// checkTPM 基于加权 fixed-minute 的 sliding counter 算法:
// 1. 同时读取当前分钟桶和上一分钟桶的 token 累计值
// 2. weighted = prev * (60 - elapsed_in_current)/60 + current
// 3. 若 weighted + EstimatedTokens > MaxTPM 则超限
// 4. 否则 INCRBY 当前桶 EstimatedTokens 并续 120s 过期
// O(1)，对 TPM 场景精度足够；边界漂移在 token 大数量下可忽略。
func (l *RedisRateLimiter) checkTPM(ctx context.Context, rule *domain.RateLimitRule, scope domain.Scope, scopeVal string, estimated int64, now time.Time) error {
	curBucket := now.Unix() / 60
	prevBucket := curBucket - 1
	curKey := tpmKey(scope, scopeVal, curBucket)
	prevKey := tpmKey(scope, scopeVal, prevBucket)

	pipe := l.rdb.Pipeline()
	curCmd := pipe.Get(ctx, curKey)
	prevCmd := pipe.Get(ctx, prevKey)
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		logger.L.Warnw("tpm pipeline exec failed", "key", curKey, "error", err)
		return nil // 降级放行
	}

	curVal := parseInt64OrZero(curCmd.Val())
	prevVal := parseInt64OrZero(prevCmd.Val())

	elapsed := now.Unix() - curBucket*60 // [0, 59]
	remainPrevRatio := float64(60-elapsed) / 60.0
	weighted := int64(float64(prevVal)*remainPrevRatio) + curVal

	if weighted+estimated > int64(rule.MaxTPM) {
		return &domain.RateLimitError{Scope: scope, Reason: domain.ReasonTPM}
	}

	pipe2 := l.rdb.Pipeline()
	pipe2.IncrBy(ctx, curKey, estimated)
	pipe2.Expire(ctx, curKey, 120*time.Second)
	if _, err := pipe2.Exec(ctx); err != nil {
		logger.L.Warnw("tpm incrby failed", "key", curKey, "error", err)
	}
	return nil
}

func parseInt64OrZero(s string) int64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func (l *RedisRateLimiter) findRules(rules []*domain.RateLimitRule, scope domain.Scope, scopeVal string) []*domain.RateLimitRule {
	var matched []*domain.RateLimitRule
	for _, r := range rules {
		if r.Scope != scope {
			continue
		}
		if scope == domain.ScopeGlobal || r.ScopeValue == scopeVal {
			matched = append(matched, r)
		}
	}
	return matched
}

func scopeValueFromReq(scope domain.Scope, req *domain.CheckRequest) string {
	switch scope {
	case domain.ScopeGlobal:
		return "global"
	case domain.ScopeUser:
		if req.UserID == 0 {
			return ""
		}
		return strconv.FormatInt(req.UserID, 10)
	case domain.ScopeAPIKey:
		if req.APIKeyID == 0 {
			return ""
		}
		return strconv.FormatInt(req.APIKeyID, 10)
	case domain.ScopeModel:
		return req.Model
	default:
		return ""
	}
}

func rpmKey(scope domain.Scope, val string) string {
	return fmt.Sprintf("rl:rpm:%s:%s", scope, val)
}

func concurrentKey(scope domain.Scope, val string) string {
	return fmt.Sprintf("rl:concurrent:%s:%s", scope, val)
}

func tpmKey(scope domain.Scope, val string, minuteBucket int64) string {
	return fmt.Sprintf("rl:tpm:%s:%s:%d", scope, val, minuteBucket)
}
