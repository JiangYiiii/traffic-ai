// Package redis 提供 Redis 连接池初始化。
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
)

func NewClient(cfg *config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
