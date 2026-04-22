package mysql

import (
	"context"
	"database/sql"
	"fmt"

	monitor "github.com/trailyai/traffic-ai/internal/domain/monitor"
)

// MonitorRepo 负责 usage_logs 的聚合查询，为监控端提供数据。
type MonitorRepo struct {
	db *sql.DB
}

func NewMonitorRepo(db *sql.DB) *MonitorRepo {
	return &MonitorRepo{db: db}
}

// ModelStat 单模型聚合行。
type ModelStat struct {
	Model          string
	TotalRequests  int64
	ErrorCount     int64
	AvgLatencyMs   float64
	P95LatencyMs   float64
	TotalTokens    int64
	TotalCostMicro int64
}

// ModelAccountStat 单账号（model_accounts 维度）聚合行。
type ModelAccountStat struct {
	ModelAccountID int64
	TotalRequests  int64
	ErrorCount     int64
	AvgLatencyMs   float64
	TotalTokens    int64
	TotalCostMicro int64
}

// TimeSeriesRow 时间桶聚合行。
type TimeSeriesRow struct {
	Bucket        string
	TotalRequests int64
	ErrorCount    int64
	AvgLatencyMs  float64
	TotalTokens   int64
}

// ListModelStats 按模型聚合最近 hours 小时的指标，用于总览。
func (r *MonitorRepo) ListModelStats(ctx context.Context, hours int) ([]*ModelStat, error) {
	const q = `
SELECT model,
       COUNT(*) AS total_requests,
       SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_count,
       AVG(latency_ms) AS avg_latency_ms,
       SUM(total_tokens) AS total_tokens,
       SUM(cost_micro_usd) AS total_cost_micro
FROM usage_logs
WHERE created_at >= NOW() - INTERVAL ? HOUR
GROUP BY model
ORDER BY total_requests DESC`

	rows, err := r.db.QueryContext(ctx, q, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ModelStat
	for rows.Next() {
		s := &ModelStat{}
		if err := rows.Scan(&s.Model, &s.TotalRequests, &s.ErrorCount,
			&s.AvgLatencyMs, &s.TotalTokens, &s.TotalCostMicro); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// GetModelP95Latency 近似 p95 延迟：取排序后第 95% 行（简化实现，MySQL 8 无原生 PERCENTILE_CONT）。
func (r *MonitorRepo) GetModelP95Latency(ctx context.Context, modelName string, hours int) (float64, error) {
	var total int64
	countQ := `SELECT COUNT(*) FROM usage_logs WHERE model = ? AND created_at >= NOW() - INTERVAL ? HOUR`
	if err := r.db.QueryRowContext(ctx, countQ, modelName, hours).Scan(&total); err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}

	offset := int64(float64(total) * 0.95)
	if offset >= total {
		offset = total - 1
	}

	var p95 float64
	dataQ := `SELECT latency_ms FROM usage_logs
		WHERE model = ? AND created_at >= NOW() - INTERVAL ? HOUR
		ORDER BY latency_ms LIMIT 1 OFFSET ?`
	if err := r.db.QueryRowContext(ctx, dataQ, modelName, hours, offset).Scan(&p95); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return p95, nil
}

// ListModelAccountStatsByModel 按模型账号聚合指定模型最近 hours 小时的指标。
//
// 注意：这里不再过滤 model_account_id > 0。
// 000011 migration 后，所有 usage_logs 行都应关联到一个有效的 model_account_id；
// 如果出现 0 就说明代码写入路径漏了，必须在 gateway 层补齐，而不是在这里偷偷吞掉。
// 吞掉=监控页骗人；暴露=问题立刻可见。
func (r *MonitorRepo) ListModelAccountStatsByModel(ctx context.Context, modelName string, hours int) ([]*ModelAccountStat, error) {
	const q = `
SELECT model_account_id,
       COUNT(*) AS total_requests,
       SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_count,
       AVG(latency_ms) AS avg_latency_ms,
       SUM(total_tokens) AS total_tokens,
       SUM(cost_micro_usd) AS total_cost_micro
FROM usage_logs
WHERE model = ? AND created_at >= NOW() - INTERVAL ? HOUR
GROUP BY model_account_id
ORDER BY total_requests DESC`

	rows, err := r.db.QueryContext(ctx, q, modelName, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ModelAccountStat
	for rows.Next() {
		s := &ModelAccountStat{}
		if err := rows.Scan(&s.ModelAccountID, &s.TotalRequests, &s.ErrorCount,
			&s.AvgLatencyMs, &s.TotalTokens, &s.TotalCostMicro); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// GetModelAccountStat 单账号最近 hours 小时的聚合指标（跨模型汇总）。
func (r *MonitorRepo) GetModelAccountStat(ctx context.Context, modelAccountID int64, hours int) (*ModelAccountStat, error) {
	const q = `
SELECT model_account_id,
       COUNT(*) AS total_requests,
       SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_count,
       AVG(latency_ms) AS avg_latency_ms,
       SUM(total_tokens) AS total_tokens,
       SUM(cost_micro_usd) AS total_cost_micro
FROM usage_logs
WHERE model_account_id = ? AND created_at >= NOW() - INTERVAL ? HOUR
GROUP BY model_account_id`

	s := &ModelAccountStat{}
	err := r.db.QueryRowContext(ctx, q, modelAccountID, hours).Scan(
		&s.ModelAccountID, &s.TotalRequests, &s.ErrorCount,
		&s.AvgLatencyMs, &s.TotalTokens, &s.TotalCostMicro,
	)
	if err == sql.ErrNoRows {
		s.ModelAccountID = modelAccountID
		return s, nil
	}
	return s, err
}

// timeBucketFmt 根据粒度返回 DATE_FORMAT 格式串。
func timeBucketFmt(granularity string) string {
	if granularity == "min" {
		return "%Y-%m-%d %H:%i"
	}
	return "%Y-%m-%d %H:00"
}

// ListModelTimeSeries 按时间桶聚合单模型的趋势数据。
func (r *MonitorRepo) ListModelTimeSeries(ctx context.Context, modelName string, hours int, granularity string) ([]*TimeSeriesRow, error) {
	bucketFmt := timeBucketFmt(granularity)
	q := fmt.Sprintf(`
SELECT DATE_FORMAT(created_at, '%s') AS bucket,
       COUNT(*) AS total_requests,
       SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_count,
       AVG(latency_ms) AS avg_latency_ms,
       SUM(total_tokens) AS total_tokens
FROM usage_logs
WHERE model = ? AND created_at >= NOW() - INTERVAL ? HOUR
GROUP BY bucket
ORDER BY bucket`, bucketFmt)

	return r.queryTimeSeries(ctx, q, modelName, hours)
}

// ListModelAccountTimeSeries 按时间桶聚合单账号的趋势数据。
func (r *MonitorRepo) ListModelAccountTimeSeries(ctx context.Context, modelAccountID int64, hours int, granularity string) ([]*TimeSeriesRow, error) {
	if modelAccountID <= 0 {
		return nil, nil
	}
	bucketFmt := timeBucketFmt(granularity)
	q := fmt.Sprintf(`
SELECT DATE_FORMAT(created_at, '%s') AS bucket,
       COUNT(*) AS total_requests,
       SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_count,
       AVG(latency_ms) AS avg_latency_ms,
       SUM(total_tokens) AS total_tokens
FROM usage_logs
WHERE model_account_id = ? AND created_at >= NOW() - INTERVAL ? HOUR
GROUP BY bucket
ORDER BY bucket`, bucketFmt)

	return r.queryTimeSeries(ctx, q, modelAccountID, hours)
}

func (r *MonitorRepo) queryTimeSeries(ctx context.Context, q string, arg interface{}, hours int) ([]*TimeSeriesRow, error) {
	rows, err := r.db.QueryContext(ctx, q, arg, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*TimeSeriesRow
	for rows.Next() {
		s := &TimeSeriesRow{}
		if err := rows.Scan(&s.Bucket, &s.TotalRequests, &s.ErrorCount, &s.AvgLatencyMs, &s.TotalTokens); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// ListAllModelNames 取 usage_logs 中最近 hours 小时出现过的模型名列表。
func (r *MonitorRepo) ListAllModelNames(ctx context.Context, hours int) ([]string, error) {
	const q = `SELECT DISTINCT model FROM usage_logs WHERE created_at >= NOW() - INTERVAL ? HOUR`
	rows, err := r.db.QueryContext(ctx, q, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// ListAllModelAccountIDs 取 usage_logs 中最近 hours 小时出现过的模型账号 ID 列表。
// 仍然过滤 >0 是因为老的历史记录里可能留有 model_account_id=0，这些不应该出现在"账号维度"的列表里。
func (r *MonitorRepo) ListAllModelAccountIDs(ctx context.Context, hours int) ([]int64, error) {
	const q = `SELECT DISTINCT model_account_id FROM usage_logs WHERE model_account_id > 0 AND created_at >= NOW() - INTERVAL ? HOUR`
	rows, err := r.db.QueryContext(ctx, q, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// toMonitorTimeSeries 把 MySQL 行转换为 domain 类型。
func toMonitorTimeSeries(rows []*TimeSeriesRow) []*monitor.TimeSeriesPoint {
	out := make([]*monitor.TimeSeriesPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, &monitor.TimeSeriesPoint{
			Bucket:        r.Bucket,
			TotalRequests: r.TotalRequests,
			ErrorCount:    r.ErrorCount,
			AvgLatencyMs:  r.AvgLatencyMs,
			TotalTokens:   r.TotalTokens,
		})
	}
	return out
}

var _ = toMonitorTimeSeries
