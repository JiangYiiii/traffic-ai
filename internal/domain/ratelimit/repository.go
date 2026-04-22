package ratelimit

import "context"

type RuleRepository interface {
	Create(ctx context.Context, rule *RateLimitRule) error
	Update(ctx context.Context, rule *RateLimitRule) error
	Delete(ctx context.Context, id int64) error
	FindByID(ctx context.Context, id int64) (*RateLimitRule, error)
	ListAll(ctx context.Context) ([]*RateLimitRule, error)
	ListActive(ctx context.Context) ([]*RateLimitRule, error)
}
