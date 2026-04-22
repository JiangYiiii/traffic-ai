package redis

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const balanceKeyPrefix = "balance:"

type BalanceCache struct {
	rdb *redis.Client
}

func NewBalanceCache(rdb *redis.Client) *BalanceCache {
	return &BalanceCache{rdb: rdb}
}

func balanceKey(userID int64) string {
	return fmt.Sprintf("%s%d", balanceKeyPrefix, userID)
}

// Get 获取缓存余额，key 不存在时返回 (0, false, nil)。
func (c *BalanceCache) Get(ctx context.Context, userID int64) (int64, bool, error) {
	val, err := c.rdb.Get(ctx, balanceKey(userID)).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	balance, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return balance, true, nil
}

// Set 设置缓存余额（无过期时间，随充值/扣费主动更新）。
func (c *BalanceCache) Set(ctx context.Context, userID, balance int64) error {
	return c.rdb.Set(ctx, balanceKey(userID), balance, 0).Err()
}

// DecrBy 原子扣减余额。扣减后余额若为负则自动回滚并返回 error。
func (c *BalanceCache) DecrBy(ctx context.Context, userID, amount int64) (int64, error) {
	newVal, err := c.rdb.DecrBy(ctx, balanceKey(userID), amount).Result()
	if err != nil {
		return 0, err
	}
	if newVal < 0 {
		c.rdb.IncrBy(ctx, balanceKey(userID), amount)
		return 0, fmt.Errorf("insufficient balance in cache")
	}
	return newVal, nil
}

// IncrBy 原子增加余额（充值 / 退款）。
func (c *BalanceCache) IncrBy(ctx context.Context, userID, amount int64) (int64, error) {
	return c.rdb.IncrBy(ctx, balanceKey(userID), amount).Result()
}

// Delete 删除缓存（用于一致性修复场景）。
func (c *BalanceCache) Delete(ctx context.Context, userID int64) error {
	return c.rdb.Del(ctx, balanceKey(userID)).Err()
}
