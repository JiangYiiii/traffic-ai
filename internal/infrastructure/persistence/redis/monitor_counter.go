package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// MonitorCounter 监控实时计数器，基于 Redis 按日分区 key。
// key 格式: mon:{scope}:{id}:{metric}:{YYYYMMDD}
// TTL: 48h，自动过期，无需手动清理。
type MonitorCounter struct {
	rdb *redis.Client
}

func NewMonitorCounter(rdb *redis.Client) *MonitorCounter {
	return &MonitorCounter{rdb: rdb}
}

// modelKey 生成模型维度 key。
func modelKey(modelName, metric, date string) string {
	return fmt.Sprintf("mon:model:%s:%s:%s", modelName, metric, date)
}

// accountKey 生成账号维度 key。
func accountKey(accountID int64, metric, date string) string {
	return fmt.Sprintf("mon:acct:%d:%s:%s", accountID, metric, date)
}

// today 返回当前日期字符串 YYYYMMDD。
func today() string {
	return time.Now().Format("20060102")
}

// MonitorEvent 单次请求的监控数据。
type MonitorEvent struct {
	ModelName   string
	AccountID   int64
	IsError     bool
	TotalTokens int
	LatencyMs   int
}

// Record 原子写入一次请求的所有监控计数。
// 使用 Pipeline 批量执行，减少 RTT。
func (c *MonitorCounter) Record(ctx context.Context, e MonitorEvent) {
	date := today()
	ttl := 48 * time.Hour

	pipe := c.rdb.Pipeline()

	// 模型维度
	mReqKey := modelKey(e.ModelName, "req", date)
	pipe.Incr(ctx, mReqKey)
	pipe.Expire(ctx, mReqKey, ttl)

	if e.IsError {
		mErrKey := modelKey(e.ModelName, "err", date)
		pipe.Incr(ctx, mErrKey)
		pipe.Expire(ctx, mErrKey, ttl)
	}

	mTokensKey := modelKey(e.ModelName, "tokens", date)
	pipe.IncrBy(ctx, mTokensKey, int64(e.TotalTokens))
	pipe.Expire(ctx, mTokensKey, ttl)

	mLatencyKey := modelKey(e.ModelName, "latency_sum", date)
	pipe.IncrBy(ctx, mLatencyKey, int64(e.LatencyMs))
	pipe.Expire(ctx, mLatencyKey, ttl)

	// 账号维度
	if e.AccountID > 0 {
		aReqKey := accountKey(e.AccountID, "req", date)
		pipe.Incr(ctx, aReqKey)
		pipe.Expire(ctx, aReqKey, ttl)

		if e.IsError {
			aErrKey := accountKey(e.AccountID, "err", date)
			pipe.Incr(ctx, aErrKey)
			pipe.Expire(ctx, aErrKey, ttl)
		}

		aTokensKey := accountKey(e.AccountID, "tokens", date)
		pipe.IncrBy(ctx, aTokensKey, int64(e.TotalTokens))
		pipe.Expire(ctx, aTokensKey, ttl)

		aLatencyKey := accountKey(e.AccountID, "latency_sum", date)
		pipe.IncrBy(ctx, aLatencyKey, int64(e.LatencyMs))
		pipe.Expire(ctx, aLatencyKey, ttl)
	}

	pipe.Exec(ctx) //nolint:errcheck
}

// ModelDayStats 单模型今日实时统计。
type ModelDayStats struct {
	ModelName      string
	TotalRequests  int64
	ErrorCount     int64
	TotalTokens    int64
	AvgLatencyMs   float64
}

// AccountDayStats 单账号今日实时统计。
type AccountDayStats struct {
	AccountID     int64
	TotalRequests int64
	ErrorCount    int64
	TotalTokens   int64
	AvgLatencyMs  float64
}

// GetModelStats 批量获取指定模型今日实时统计。
func (c *MonitorCounter) GetModelStats(ctx context.Context, modelNames []string) (map[string]*ModelDayStats, error) {
	if len(modelNames) == 0 {
		return map[string]*ModelDayStats{}, nil
	}
	date := today()
	result := make(map[string]*ModelDayStats, len(modelNames))

	// 一次 pipeline 取所有 key
	pipe := c.rdb.Pipeline()
	type cmds struct {
		req     *redis.StringCmd
		err     *redis.StringCmd
		tokens  *redis.StringCmd
		latency *redis.StringCmd
	}
	all := make([]cmds, len(modelNames))
	for i, name := range modelNames {
		all[i] = cmds{
			req:     pipe.Get(ctx, modelKey(name, "req", date)),
			err:     pipe.Get(ctx, modelKey(name, "err", date)),
			tokens:  pipe.Get(ctx, modelKey(name, "tokens", date)),
			latency: pipe.Get(ctx, modelKey(name, "latency_sum", date)),
		}
	}
	pipe.Exec(ctx) //nolint:errcheck

	for i, name := range modelNames {
		stats := &ModelDayStats{ModelName: name}
		stats.TotalRequests = parseInt64(all[i].req)
		stats.ErrorCount = parseInt64(all[i].err)
		stats.TotalTokens = parseInt64(all[i].tokens)
		latencySum := parseInt64(all[i].latency)
		if stats.TotalRequests > 0 {
			stats.AvgLatencyMs = float64(latencySum) / float64(stats.TotalRequests)
		}
		result[name] = stats
	}
	return result, nil
}

// GetAccountStats 批量获取指定账号今日实时统计。
func (c *MonitorCounter) GetAccountStats(ctx context.Context, accountIDs []int64) (map[int64]*AccountDayStats, error) {
	if len(accountIDs) == 0 {
		return map[int64]*AccountDayStats{}, nil
	}
	date := today()
	result := make(map[int64]*AccountDayStats, len(accountIDs))

	pipe := c.rdb.Pipeline()
	type cmds struct {
		req     *redis.StringCmd
		err     *redis.StringCmd
		tokens  *redis.StringCmd
		latency *redis.StringCmd
	}
	all := make([]cmds, len(accountIDs))
	for i, id := range accountIDs {
		all[i] = cmds{
			req:     pipe.Get(ctx, accountKey(id, "req", date)),
			err:     pipe.Get(ctx, accountKey(id, "err", date)),
			tokens:  pipe.Get(ctx, accountKey(id, "tokens", date)),
			latency: pipe.Get(ctx, accountKey(id, "latency_sum", date)),
		}
	}
	pipe.Exec(ctx) //nolint:errcheck

	for i, id := range accountIDs {
		stats := &AccountDayStats{AccountID: id}
		stats.TotalRequests = parseInt64(all[i].req)
		stats.ErrorCount = parseInt64(all[i].err)
		stats.TotalTokens = parseInt64(all[i].tokens)
		latencySum := parseInt64(all[i].latency)
		if stats.TotalRequests > 0 {
			stats.AvgLatencyMs = float64(latencySum) / float64(stats.TotalRequests)
		}
		result[id] = stats
	}
	return result, nil
}

func parseInt64(cmd *redis.StringCmd) int64 {
	if cmd.Err() != nil {
		return 0
	}
	v, _ := strconv.ParseInt(cmd.Val(), 10, 64)
	return v
}
