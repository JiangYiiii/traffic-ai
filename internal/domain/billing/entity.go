package billing

import "time"

type Balance struct {
	UserID         int64
	Balance        int64
	TotalCharged   int64
	TotalConsumed  int64
	AlertEnabled   bool
	AlertThreshold int64
	UpdatedAt      time.Time
}

type BalanceLog struct {
	ID            int64
	UserID        int64
	Amount        int64
	BalanceBefore int64
	BalanceAfter  int64
	ReasonType    string
	ReasonDetail  string
	RequestID     string
	CreatedAt     time.Time
}

type RedeemCode struct {
	ID        int64
	Code      string
	Amount    int64
	Status    int
	UsedBy    *int64
	UsedAt    *time.Time
	CreatedBy int64
	CreatedAt time.Time
}

const (
	ReasonCharge  = "charge"
	ReasonConsume = "consume"
	ReasonRedeem  = "redeem"
	ReasonAdjust  = "adjust"
	ReasonRefund  = "refund"

	RedeemStatusUnused = 0
	RedeemStatusUsed   = 1
)
