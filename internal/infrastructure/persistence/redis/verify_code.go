package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// @ai_doc VerifyCodeStore: 验证码 Redis 存储，key 格式 verify_code:{purpose}:{email}，TTL 5分钟
type VerifyCodeStore struct {
	rdb *redis.Client
}

func NewVerifyCodeStore(rdb *redis.Client) *VerifyCodeStore {
	return &VerifyCodeStore{rdb: rdb}
}

func (s *VerifyCodeStore) Save(ctx context.Context, purpose, email, code string) error {
	key := verifyCodeKey(purpose, email)
	return s.rdb.Set(ctx, key, code, 5*time.Minute).Err()
}

// @ai_doc_rule Verify: 验证码匹配后立即删除，防止重放
func (s *VerifyCodeStore) Verify(ctx context.Context, purpose, email, code string) (bool, error) {
	key := verifyCodeKey(purpose, email)
	stored, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if stored != code {
		return false, nil
	}
	s.rdb.Del(ctx, key)
	return true, nil
}

func verifyCodeKey(purpose, email string) string {
	return fmt.Sprintf("verify_code:%s:%s", purpose, email)
}

// @ai_doc LoginLockStore: 登录失败计数 Redis 存储，5次失败锁定15分钟
type LoginLockStore struct {
	rdb *redis.Client
}

func NewLoginLockStore(rdb *redis.Client) *LoginLockStore {
	return &LoginLockStore{rdb: rdb}
}

const (
	lockThreshold = 5
	lockTTL       = 15 * time.Minute
)

// @ai_doc_rule IncrFailCount: INCR + 首次设 TTL，确保窗口自动过期
func (s *LoginLockStore) IncrFailCount(ctx context.Context, email string) (int64, error) {
	key := loginFailKey(email)
	cnt, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if cnt == 1 {
		s.rdb.Expire(ctx, key, lockTTL)
	}
	return cnt, nil
}

func (s *LoginLockStore) IsLocked(ctx context.Context, email string) (bool, error) {
	key := loginFailKey(email)
	cnt, err := s.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return cnt >= lockThreshold, nil
}

func (s *LoginLockStore) Reset(ctx context.Context, email string) error {
	return s.rdb.Del(ctx, loginFailKey(email)).Err()
}

func loginFailKey(email string) string {
	return fmt.Sprintf("login_fail:%s", email)
}
