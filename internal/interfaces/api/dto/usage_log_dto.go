package dto

import (
	"fmt"

	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
)

// UsageLogItem 是 /admin/usage-logs 使用的管理端 DTO。保持 snake_case 以兼容已有前端脚本。
type UsageLogItem struct {
	ID           int64  `json:"id"`
	RequestID    string `json:"request_id"`
	UserID       int64  `json:"user_id"`
	APIKeyID     int64  `json:"api_key_id"`
	Model        string `json:"model"`
	Protocol     string `json:"protocol"`
	IsStream     bool   `json:"is_stream"`
	Status       string `json:"status"`
	Error        string `json:"error_message,omitempty"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	CostMicroUSD int64  `json:"cost_micro_usd"`
	LatencyMs    int    `json:"latency_ms"`
	CreatedAt    string `json:"created_at"`
}

func ToUsageLogItem(l *mysql.UsageLog) UsageLogItem {
	return UsageLogItem{
		ID:           l.ID,
		RequestID:    l.RequestID,
		UserID:       l.UserID,
		APIKeyID:     l.APIKeyID,
		Model:        l.Model,
		Protocol:     l.Protocol,
		IsStream:     l.IsStream,
		Status:       l.Status,
		Error:        l.ErrorMessage,
		InputTokens:  l.InputTokens,
		OutputTokens: l.OutputTokens,
		TotalTokens:  l.TotalTokens,
		CostMicroUSD: l.CostMicroUSD,
		LatencyMs:    l.LatencyMs,
		CreatedAt:    l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func ToUsageLogList(logs []*mysql.UsageLog) []UsageLogItem {
	out := make([]UsageLogItem, 0, len(logs))
	for _, l := range logs {
		out = append(out, ToUsageLogItem(l))
	}
	return out
}

// UserUsageLogItem 是用户控制台 /me/usage-logs 的 DTO。字段为 camelCase，
// 与 demo/userClient/js/app.js 的 renderer 对齐：time/type/tokenName/tokenGroup/...
type UserUsageLogItem struct {
	ID                  int64  `json:"id"`
	RequestID           string `json:"requestId"`
	Time                string `json:"time"`
	Type                string `json:"type"`
	TokenName           string `json:"tokenName"`
	TokenGroup          string `json:"tokenGroup"`
	Model               string `json:"model"`
	ReasoningEffort     string `json:"reasoningEffort"`
	LatencyMs           int    `json:"latencyMs"`
	Stream              bool   `json:"stream"`
	Status              string `json:"status"`
	PromptTokens        int    `json:"promptTokens"`
	CompletionTokens    int    `json:"completionTokens"`
	ReasoningTokens     int    `json:"reasoningTokens"`
	CacheCreationTokens int    `json:"cacheCreationTokens"`
	CacheReadTokens     int    `json:"cacheReadTokens"`
	TotalTokens         int    `json:"totalTokens"`
	CostMicroUsd        int64  `json:"costMicroUsd"`
	CostUsdApprox       string `json:"costUsdApprox"`
	IP                  string `json:"ip"`
	Note                string `json:"note"`
}

// ToUserUsageLogItem 把仓储层模型映射成用户控制台 DTO。
// costUsdApprox = costMicroUsd / 1e6，保留 6 位小数，空值友好 (0 → "0.000000")。
func ToUserUsageLogItem(l *mysql.UsageLog) UserUsageLogItem {
	return UserUsageLogItem{
		ID:                  l.ID,
		RequestID:           l.RequestID,
		Time:                l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Type:                l.Protocol,
		TokenName:           l.JoinTokenName,
		TokenGroup:          l.JoinTokenGroup,
		Model:               l.Model,
		ReasoningEffort:     l.ReasoningEffort,
		LatencyMs:           l.LatencyMs,
		Stream:              l.IsStream,
		Status:              l.Status,
		PromptTokens:        l.InputTokens,
		CompletionTokens:    l.OutputTokens,
		ReasoningTokens:     l.ReasoningTokens,
		CacheCreationTokens: l.CacheCreationTokens,
		CacheReadTokens:     l.CacheReadTokens,
		TotalTokens:         l.TotalTokens,
		CostMicroUsd:        l.CostMicroUSD,
		CostUsdApprox:       fmt.Sprintf("%.6f", float64(l.CostMicroUSD)/1e6),
		IP:                  l.ClientIP,
		Note:                l.Note,
	}
}

func ToUserUsageLogList(logs []*mysql.UsageLog) []UserUsageLogItem {
	out := make([]UserUsageLogItem, 0, len(logs))
	for _, l := range logs {
		out = append(out, ToUserUsageLogItem(l))
	}
	return out
}
