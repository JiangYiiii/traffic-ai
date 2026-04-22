package billing

import "context"

// BillingService 供网关调用的扣费接口
// @ai_doc_flow 预扣费 → 最终结算 → 多退少补
type BillingService interface {
	PreDeduct(ctx context.Context, userID int64, estimatedMicroUSD int64, requestID string) error
	Settle(ctx context.Context, userID int64, actualMicroUSD int64, preDeducted int64, requestID string, detail string) error
	CheckBalance(ctx context.Context, userID int64, requiredMicroUSD int64) error
	InitBalance(ctx context.Context, userID int64) error
}
