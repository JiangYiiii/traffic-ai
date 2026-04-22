// Package monitor 定义流量监控聚合结果的领域类型。
package monitor

import "time"

// ModelOverview 单模型在指定时间段内的聚合指标。
type ModelOverview struct {
	ModelID        int64   `json:"model_id"`
	ModelName      string  `json:"model_name"`
	TotalRequests  int64   `json:"total_requests"`
	ErrorCount     int64   `json:"error_count"`
	ErrorRate      float64 `json:"error_rate"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	P95LatencyMs   float64 `json:"p95_latency_ms"`
	TotalTokens    int64   `json:"total_tokens"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	ActiveAccounts int     `json:"active_accounts"`
	// 今日 Redis 实时指标（当日累计）
	TodayRequests  int64   `json:"today_requests"`
	TodayTokens    int64   `json:"today_tokens"`
}

// AccountMetrics 单账号在指定时间段内的聚合指标。
type AccountMetrics struct {
	AccountID    int64   `json:"account_id"`
	AccountName  string  `json:"account_name"`
	Provider     string  `json:"provider"`
	Status       string  `json:"status"`
	TotalRequests int64  `json:"total_requests"`
	ErrorCount   int64   `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	// 今日 Redis 实时指标
	TodayRequests int64   `json:"today_requests"`
	TodayTokens   int64   `json:"today_tokens"`
	TodayAvgLatency float64 `json:"today_avg_latency_ms"`
}

// TimeSeriesPoint 时间趋势中的单个桶数据点。
type TimeSeriesPoint struct {
	Bucket        string  `json:"bucket"`
	TotalRequests int64   `json:"total_requests"`
	ErrorCount    int64   `json:"error_count"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	TotalTokens   int64   `json:"total_tokens"`
}

// OverviewResult overview 接口的完整响应。
type OverviewResult struct {
	Models      []*ModelOverview `json:"models"`
	PeriodHours int              `json:"period_hours"`
	GeneratedAt time.Time        `json:"generated_at"`
}

// ModelDetailResult 单模型下钻接口的完整响应。
type ModelDetailResult struct {
	Model       *ModelOverview    `json:"model"`
	Accounts    []*AccountMetrics `json:"accounts"`
	TimeSeries  []*TimeSeriesPoint `json:"time_series"`
	PeriodHours int               `json:"period_hours"`
	Granularity string            `json:"granularity"`
}

// AccountDetailResult 单账号下钻接口的完整响应。
type AccountDetailResult struct {
	Account     *AccountMetrics    `json:"account"`
	TimeSeries  []*TimeSeriesPoint `json:"time_series"`
	PeriodHours int                `json:"period_hours"`
	Granularity string             `json:"granularity"`
}

// RealtimeResult Redis 实时快照。
type RealtimeResult struct {
	Models      []*ModelRealtimeStats   `json:"models"`
	Accounts    []*AccountRealtimeStats `json:"accounts"`
	Date        string                  `json:"date"`
}

// ModelRealtimeStats 模型今日实时计数。
type ModelRealtimeStats struct {
	ModelName     string  `json:"model_name"`
	TodayRequests int64   `json:"today_requests"`
	TodayErrors   int64   `json:"today_errors"`
	TodayTokens   int64   `json:"today_tokens"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// AccountRealtimeStats 账号今日实时计数。
type AccountRealtimeStats struct {
	AccountID     int64   `json:"account_id"`
	AccountName   string  `json:"account_name"`
	TodayRequests int64   `json:"today_requests"`
	TodayErrors   int64   `json:"today_errors"`
	TodayTokens   int64   `json:"today_tokens"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}
