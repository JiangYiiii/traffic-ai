package ratelimit

import (
	"context"
	"sync"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/ratelimit"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

// UseCase 提供规则 CRUD 和限流检查编排。
// 内部维护活跃规则的内存缓存，每 30 秒从 MySQL 轮询刷新。
type UseCase struct {
	repo    domain.RuleRepository
	limiter domain.RateLimiter

	mu    sync.RWMutex
	cache []*domain.RateLimitRule

	stopOnce sync.Once
	stopCh   chan struct{}
}

func NewUseCase(repo domain.RuleRepository, limiter domain.RateLimiter) *UseCase {
	uc := &UseCase{
		repo:    repo,
		limiter: limiter,
		stopCh:  make(chan struct{}),
	}
	uc.loadRules()
	go uc.refreshLoop()
	return uc
}

// ActiveRules 返回当前内存中的活跃规则快照，供 RedisRateLimiter 调用。
func (uc *UseCase) ActiveRules() []*domain.RateLimitRule {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.cache
}

// Stop 终止后台轮询。
func (uc *UseCase) Stop() {
	uc.stopOnce.Do(func() { close(uc.stopCh) })
}

// ---- 限流检查（网关调用） ----

func (uc *UseCase) Allow(ctx context.Context, req *domain.CheckRequest) error {
	return uc.limiter.Allow(ctx, req)
}

func (uc *UseCase) Release(ctx context.Context, req *domain.CheckRequest) {
	uc.limiter.Release(ctx, req)
}

// ---- 管理后台 CRUD ----

func (uc *UseCase) Create(ctx context.Context, rule *domain.RateLimitRule) error {
	if err := uc.repo.Create(ctx, rule); err != nil {
		logger.L.Errorw("create rate limit rule failed", "error", err)
		return errcode.ErrInternal
	}
	uc.loadRules()
	return nil
}

func (uc *UseCase) Update(ctx context.Context, rule *domain.RateLimitRule) error {
	existing, err := uc.repo.FindByID(ctx, rule.ID)
	if err != nil {
		logger.L.Errorw("find rate limit rule failed", "error", err, "id", rule.ID)
		return errcode.ErrInternal
	}
	if existing == nil {
		return errcode.ErrNotFound
	}
	if err := uc.repo.Update(ctx, rule); err != nil {
		logger.L.Errorw("update rate limit rule failed", "error", err, "id", rule.ID)
		return errcode.ErrInternal
	}
	uc.loadRules()
	return nil
}

func (uc *UseCase) Delete(ctx context.Context, id int64) error {
	existing, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		logger.L.Errorw("find rate limit rule failed", "error", err, "id", id)
		return errcode.ErrInternal
	}
	if existing == nil {
		return errcode.ErrNotFound
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		logger.L.Errorw("delete rate limit rule failed", "error", err, "id", id)
		return errcode.ErrInternal
	}
	uc.loadRules()
	return nil
}

func (uc *UseCase) List(ctx context.Context) ([]*domain.RateLimitRule, error) {
	rules, err := uc.repo.ListAll(ctx)
	if err != nil {
		logger.L.Errorw("list rate limit rules failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return rules, nil
}

func (uc *UseCase) FindByID(ctx context.Context, id int64) (*domain.RateLimitRule, error) {
	rule, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		logger.L.Errorw("find rate limit rule failed", "error", err, "id", id)
		return nil, errcode.ErrInternal
	}
	if rule == nil {
		return nil, errcode.ErrNotFound
	}
	return rule, nil
}

// ---- 内部方法 ----

func (uc *UseCase) loadRules() {
	rules, err := uc.repo.ListActive(context.Background())
	if err != nil {
		logger.L.Errorw("load active rate limit rules failed", "error", err)
		return
	}
	uc.mu.Lock()
	uc.cache = rules
	uc.mu.Unlock()
	logger.L.Infow("rate limit rules reloaded", "count", len(rules))
}

func (uc *UseCase) refreshLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			uc.loadRules()
		case <-uc.stopCh:
			return
		}
	}
}
