package billing

import "context"

type BalanceRepository interface {
	Init(ctx context.Context, userID int64) error
	GetByUserID(ctx context.Context, userID int64) (*Balance, error)
	AddBalance(ctx context.Context, userID, amount int64) (*Balance, error)
	UpdateAlert(ctx context.Context, userID int64, enabled bool, threshold int64) error
}

type BalanceLogRepository interface {
	Create(ctx context.Context, log *BalanceLog) error
	ListByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*BalanceLog, int64, error)
	ListAllPaged(ctx context.Context, reasonType string, page, pageSize int) ([]*BalanceLog, int64, error)
}

type RedeemCodeRepository interface {
	Claim(ctx context.Context, code string, userID int64) (*RedeemCode, error)
	BatchCreate(ctx context.Context, codes []*RedeemCode) error
	List(ctx context.Context, page, pageSize int) ([]*RedeemCode, int64, error)
}
