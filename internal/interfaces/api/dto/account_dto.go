package dto

import (
	"fmt"

	"github.com/trailyai/traffic-ai/internal/domain/auth"
	billingDomain "github.com/trailyai/traffic-ai/internal/domain/billing"
)

// ---- User Profile ----

type ProfileResp struct {
	ID             int64  `json:"id"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	Balance        int64  `json:"balance"`
	TotalCharged   int64  `json:"total_charged"`
	TotalConsumed  int64  `json:"total_consumed"`
	AlertEnabled   bool   `json:"alert_enabled"`
	AlertThreshold int64  `json:"alert_threshold"`
}

type DashboardKPI struct {
	BalanceMicroUsd       string `json:"balanceMicroUsd"`
	TotalConsumedMicroUsd string `json:"totalConsumedMicroUsd"`
	TotalCalls            int64  `json:"totalCalls"`
	ActiveTokenCount      int64  `json:"activeTokenCount"`
}

type BalanceAlertDTO struct {
	Enabled           bool   `json:"enabled"`
	ThresholdMicroUsd string `json:"thresholdMicroUsd"`
}

type ProfileWithDashboardResp struct {
	Profile      ProfileResp     `json:"profile"`
	Dashboard    DashboardKPI    `json:"dashboard"`
	BalanceAlert BalanceAlertDTO `json:"balanceAlert"`
}

func ToProfileResp(u *auth.User, b *billingDomain.Balance) ProfileResp {
	return ProfileResp{
		ID:             u.ID,
		Email:          u.Email,
		Role:           u.Role,
		Balance:        b.Balance,
		TotalCharged:   b.TotalCharged,
		TotalConsumed:  b.TotalConsumed,
		AlertEnabled:   b.AlertEnabled,
		AlertThreshold: b.AlertThreshold,
	}
}

func ToProfileRespWithDashboard(u *auth.User, b *billingDomain.Balance, activeTokenCount, totalCalls int64) ProfileWithDashboardResp {
	return ProfileWithDashboardResp{
		Profile: ProfileResp{
			ID:             u.ID,
			Email:          u.Email,
			Role:           u.Role,
			Balance:        b.Balance,
			TotalCharged:   b.TotalCharged,
			TotalConsumed:  b.TotalConsumed,
			AlertEnabled:   b.AlertEnabled,
			AlertThreshold: b.AlertThreshold,
		},
		Dashboard: DashboardKPI{
			BalanceMicroUsd:       fmt.Sprintf("%d", b.Balance),
			TotalConsumedMicroUsd: fmt.Sprintf("%d", b.TotalConsumed),
			TotalCalls:            totalCalls,
			ActiveTokenCount:      activeTokenCount,
		},
		BalanceAlert: BalanceAlertDTO{
			Enabled:           b.AlertEnabled,
			ThresholdMicroUsd: fmt.Sprintf("%d", b.AlertThreshold),
		},
	}
}
