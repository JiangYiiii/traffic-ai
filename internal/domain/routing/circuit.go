package routing

import "context"

// CircuitBreaker 账号级熔断器接口。
//
// 由基础设施层（Redis）实现，路由层调用 Allow 过滤候选账号、
// gateway 层在上游调用结束后调用 RecordSuccess / RecordFailure 反馈结果。
//
// 状态机：closed → open → half_open → closed。
//   - closed：正常放行并统计错误率
//   - open：熔断期内拒绝放行，超过 cooldown 后自动切 half_open
//   - half_open：按概率探测放行，连续成功 N 次恢复 closed，失败则回到 open 并指数退避
type CircuitBreaker interface {
	// Allow 判断 accountID 当前是否允许调用。
	// closed / half_open（命中探测）返回 true，open 返回 false。
	Allow(ctx context.Context, accountID int64) (bool, error)

	// RecordSuccess 上报一次成功调用。
	RecordSuccess(ctx context.Context, accountID int64) error

	// RecordFailure 上报一次失败调用。kind 为错误分类字符串，由 gateway 层定义；
	// 本接口只负责统计计数与状态转换，不关心具体分类语义。
	RecordFailure(ctx context.Context, accountID int64, kind string) error

	// State 返回当前状态字符串："closed"/"open"/"half_open"，未知返回 "closed"。
	// 供 /metrics 和 admin UI 查询使用。
	State(ctx context.Context, accountID int64) (string, error)
}
