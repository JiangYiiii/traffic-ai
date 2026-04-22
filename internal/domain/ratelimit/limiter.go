package ratelimit

import (
	"context"
	"fmt"

	"github.com/trailyai/traffic-ai/pkg/errcode"
)

// CheckRequest 网关发起限流检查时的请求参数。
// EstimatedTokens 为 TPM 扣减预估量（estimateTokens 口径：input+output tokens），
// 值 <= 0 表示跳过 TPM 检查（例如按次计费模型）。
type CheckRequest struct {
	UserID          int64
	APIKeyID        int64
	Model           string
	EstimatedTokens int64
}

// RateLimiter 多级限流引擎接口，供网关直接调用。
// @ai_doc_flow 多级限流检查: 按 global → user → api_key → model 逐级检查，任一级别超限返回 error。
type RateLimiter interface {
	Allow(ctx context.Context, req *CheckRequest) error
	Release(ctx context.Context, req *CheckRequest)
}

// RateLimitError 限流拒绝时携带的结构化错误，记录命中的 scope 与触发维度（rpm/tpm/concurrent）。
// 业务层可通过 errors.As 拿到具体 reason 做指标埋点；
// Is(errcode.ErrRateLimited) 必须返回 true，兼容历史 errors.Is 判断与 HTTP 状态映射。
type RateLimitError struct {
	Scope  Scope  // 命中的限流级别：global / user / api_key / model
	Reason string // 触发维度：rpm / tpm / concurrent
}

// 合法 reason 常量，供业务层打点时统一使用。
const (
	ReasonRPM        = "rpm"
	ReasonTPM        = "tpm"
	ReasonConcurrent = "concurrent"
)

// Error 实现 error 接口；保持字符串稳定便于日志 grep。
func (e *RateLimitError) Error() string {
	if e == nil {
		return "rate limit exceeded"
	}
	return fmt.Sprintf("rate limit exceeded: scope=%s reason=%s", e.Scope, e.Reason)
}

// Is 让 errors.Is(err, errcode.ErrRateLimited) 继续成立，
// 保障现有调用方（包括 rate_limiter_test.go 的测试断言、handler 的错误分流）无需改动。
func (e *RateLimitError) Is(target error) bool {
	return target == errcode.ErrRateLimited
}
