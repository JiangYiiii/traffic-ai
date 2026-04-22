package routing

import "time"

type TokenGroup struct {
	ID          int64
	Name        string
	Description string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TokenGroupModelAccount 表示 token 分组与模型账号的多对多关系。
type TokenGroupModelAccount struct {
	ID             int64
	TokenGroupID   int64
	ModelAccountID int64
}
