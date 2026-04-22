package routing

import (
	"context"

	"github.com/trailyai/traffic-ai/internal/domain/model"
)

// RouteResult 路由结果：一条选中的模型账号 + 模型。
type RouteResult struct {
	Account *model.ModelAccount
	Model   *model.Model
}

// RoutingService selects an available model account for a given request context.
type RoutingService interface {
	// SelectModelAccount picks one active model account by tokenGroup + modelName + protocol, weighted random.
	SelectModelAccount(ctx context.Context, tokenGroup, modelName, protocol string) (*RouteResult, error)
	// SelectModelAccountExcluding 同 SelectModelAccount，但会跳过给定的 excludeIDs。
	// 用于 fallback 场景：前一个账号失败后，下次选号排除它。
	SelectModelAccountExcluding(ctx context.Context, tokenGroup, modelName, protocol string, excludeIDs []int64) (*RouteResult, error)
	// ListAvailableModels returns models accessible from the given tokenGroup (for /v1/models).
	ListAvailableModels(ctx context.Context, tokenGroup string) ([]*model.Model, error)
}
