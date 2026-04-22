package model

import "context"

// ListFilter 管理端模型列表筛选。
type ListFilter struct {
	Provider string // 精确匹配（向后兼容），空表示不限
	NameLike string // 模型名子串模糊匹配，空表示不限
}

type ModelRepository interface {
	Create(ctx context.Context, m *Model) error
	FindByID(ctx context.Context, id int64) (*Model, error)
	FindByName(ctx context.Context, name string) (*Model, error)
	List(ctx context.Context, filter ListFilter) ([]*Model, error)
	ListListedModels(ctx context.Context) ([]*Model, error) // 获取已上架的模型 (is_active=1 AND is_listed=1)
	Update(ctx context.Context, m *Model) error
	UpdateLastTest(ctx context.Context, modelID int64, success bool, latencyMs int, errMsg string) error
	Delete(ctx context.Context, id int64) error
	ListByIDs(ctx context.Context, ids []int64) ([]*Model, error)
}

// ModelAccountListFilter 模型账号列表筛选条件（跨模型检索时使用）。
type ModelAccountListFilter struct {
	ModelID  int64  // 非 0 时限定某个模型
	Provider string // 精确匹配 provider
	NameLike string // 账号名模糊
}

type ModelAccountRepository interface {
	Create(ctx context.Context, a *ModelAccount) error
	FindByID(ctx context.Context, id int64) (*ModelAccount, error)
	ListByModelID(ctx context.Context, modelID int64) ([]*ModelAccount, error)
	Update(ctx context.Context, a *ModelAccount) error
	Delete(ctx context.Context, id int64) error
	ListActiveByModelIDs(ctx context.Context, modelIDs []int64) ([]*ModelAccount, error)
	ListByIDs(ctx context.Context, ids []int64) ([]*ModelAccount, error)
	List(ctx context.Context, filter ModelAccountListFilter) ([]*ModelAccount, error)
	UpdateLastTest(ctx context.Context, accountID int64, success bool, latencyMs int, errMsg string) error
}
