package dto

import (
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/billing"
)

// ---------- Request ----------

type RedeemReq struct {
	Code string `json:"code" binding:"required"`
}

type UpdateAlertReq struct {
	Enabled   bool  `json:"enabled"`
	Threshold int64 `json:"threshold" binding:"min=0"`
}

type AdminChargeReq struct {
	Amount int64  `json:"amount" binding:"required,gt=0"`
	Detail string `json:"detail" binding:"required,max=500"`
}

type BatchRedeemCodesReq struct {
	Count  int   `json:"count" binding:"required,min=1,max=500"`
	Amount int64 `json:"amount" binding:"required,gt=0"`
}

// ---------- Response ----------

type BalanceResp struct {
	UserID         int64  `json:"user_id"`
	Balance        int64  `json:"balance"`
	TotalCharged   int64  `json:"total_charged"`
	TotalConsumed  int64  `json:"total_consumed"`
	AlertEnabled   bool   `json:"alert_enabled"`
	AlertThreshold int64  `json:"alert_threshold"`
	UpdatedAt      string `json:"updated_at"`
}

func ToBalanceResp(b *domain.Balance) BalanceResp {
	return BalanceResp{
		UserID:         b.UserID,
		Balance:        b.Balance,
		TotalCharged:   b.TotalCharged,
		TotalConsumed:  b.TotalConsumed,
		AlertEnabled:   b.AlertEnabled,
		AlertThreshold: b.AlertThreshold,
		UpdatedAt:      b.UpdatedAt.Format(time.RFC3339),
	}
}

type BalanceLogItem struct {
	ID            int64  `json:"id"`
	Amount        int64  `json:"amount"`
	BalanceBefore int64  `json:"balance_before"`
	BalanceAfter  int64  `json:"balance_after"`
	ReasonType    string `json:"reason_type"`
	ReasonDetail  string `json:"reason_detail"`
	RequestID     string `json:"request_id,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func ToBalanceLogItem(l *domain.BalanceLog) BalanceLogItem {
	return BalanceLogItem{
		ID:            l.ID,
		Amount:        l.Amount,
		BalanceBefore: l.BalanceBefore,
		BalanceAfter:  l.BalanceAfter,
		ReasonType:    l.ReasonType,
		ReasonDetail:  l.ReasonDetail,
		RequestID:     l.RequestID,
		CreatedAt:     l.CreatedAt.Format(time.RFC3339),
	}
}

func ToBalanceLogList(logs []*domain.BalanceLog) []BalanceLogItem {
	items := make([]BalanceLogItem, 0, len(logs))
	for _, l := range logs {
		items = append(items, ToBalanceLogItem(l))
	}
	return items
}

type RedeemCodeItem struct {
	ID        int64   `json:"id"`
	Code      string  `json:"code"`
	Amount    int64   `json:"amount"`
	Status    int     `json:"status"`
	UsedBy    *int64  `json:"used_by,omitempty"`
	UsedAt    *string `json:"used_at,omitempty"`
	CreatedBy int64   `json:"created_by"`
	CreatedAt string  `json:"created_at"`
}

func ToRedeemCodeItem(rc *domain.RedeemCode) RedeemCodeItem {
	item := RedeemCodeItem{
		ID:        rc.ID,
		Code:      rc.Code,
		Amount:    rc.Amount,
		Status:    rc.Status,
		UsedBy:    rc.UsedBy,
		CreatedBy: rc.CreatedBy,
		CreatedAt: rc.CreatedAt.Format(time.RFC3339),
	}
	if rc.UsedAt != nil {
		s := rc.UsedAt.Format(time.RFC3339)
		item.UsedAt = &s
	}
	return item
}

func ToRedeemCodeList(codes []*domain.RedeemCode) []RedeemCodeItem {
	items := make([]RedeemCodeItem, 0, len(codes))
	for _, c := range codes {
		items = append(items, ToRedeemCodeItem(c))
	}
	return items
}
