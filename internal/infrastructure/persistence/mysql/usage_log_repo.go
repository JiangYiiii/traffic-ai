package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type UsageLogFilter struct {
	Model          string
	Status         string
	UserID         int64
	IsStreamFilter *string // "true" or "false"
	// StartTime/EndTime 为空指针时表示不按时间过滤；
	// 非空时作用于 usage_logs.created_at 上的闭区间过滤。
	StartTime *time.Time
	EndTime   *time.Time
}

// UsageLog 对应 usage_logs 表的行结构。
type UsageLog struct {
	ID             int64
	RequestID      string
	UserID         int64
	APIKeyID       int64
	Model          string
	ModelAccountID int64 // usage_logs.model_account_id，指向 model_accounts.id
	Protocol       string
	IsStream            bool
	Status              string
	ErrorMessage        string
	InputTokens         int
	OutputTokens        int
	ReasoningTokens     int
	CacheCreationTokens int
	CacheReadTokens     int
	ReasoningEffort     string
	TotalTokens         int
	CostMicroUSD        int64
	LatencyMs           int
	ClientIP            string
	Note                string
	CreatedAt           time.Time

	// JoinTokenName / JoinTokenGroup 仅在 ListPagedWithToken 场景填充，
	// 用于用户控制台展示子令牌名与分组；持久化字段仍在 api_keys 表中。
	JoinTokenName  string
	JoinTokenGroup string
}

type UsageLogRepo struct {
	db *sql.DB
}

func NewUsageLogRepo(db *sql.DB) *UsageLogRepo {
	return &UsageLogRepo{db: db}
}

func (r *UsageLogRepo) Create(ctx context.Context, log *UsageLog) error {
	const q = `INSERT INTO usage_logs
		(request_id, user_id, api_key_id, model, model_account_id, protocol,
		 is_stream, status, error_message,
		 input_tokens, output_tokens, reasoning_tokens,
		 cache_creation_tokens, cache_read_tokens, reasoning_effort,
		 total_tokens,
		 cost_micro_usd, latency_ms, client_ip, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	isStream := 0
	if log.IsStream {
		isStream = 1
	}
	res, err := r.db.ExecContext(ctx, q,
		log.RequestID, log.UserID, log.APIKeyID, log.Model, log.ModelAccountID, log.Protocol,
		isStream, log.Status, log.ErrorMessage,
		log.InputTokens, log.OutputTokens, log.ReasoningTokens,
		log.CacheCreationTokens, log.CacheReadTokens, log.ReasoningEffort,
		log.TotalTokens,
		log.CostMicroUSD, log.LatencyMs, log.ClientIP, log.Note, log.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	log.ID = id
	return nil
}

func (r *UsageLogRepo) ListPaged(ctx context.Context, filter *UsageLogFilter, page, pageSize int) ([]*UsageLog, int64, error) {
	var where []string
	var args []interface{}

	if filter != nil {
		if filter.Model != "" {
			where = append(where, "model = ?")
			args = append(args, filter.Model)
		}
		if filter.Status != "" {
			where = append(where, "status = ?")
			args = append(args, filter.Status)
		}
		if filter.UserID > 0 {
			where = append(where, "user_id = ?")
			args = append(args, filter.UserID)
		}
		if filter.IsStreamFilter != nil {
			if *filter.IsStreamFilter == "true" {
				where = append(where, "is_stream = 1")
			} else if *filter.IsStreamFilter == "false" {
				where = append(where, "is_stream = 0")
			}
		}
		if filter.StartTime != nil {
			where = append(where, "created_at >= ?")
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			where = append(where, "created_at <= ?")
			args = append(args, *filter.EndTime)
		}
	}

	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM usage_logs" + cond
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	dataQ := fmt.Sprintf(
		`SELECT id, request_id, user_id, api_key_id, model, model_account_id, protocol,
		        is_stream, status, error_message,
		        input_tokens, output_tokens, reasoning_tokens,
		        cache_creation_tokens, cache_read_tokens, reasoning_effort,
		        total_tokens,
		        cost_micro_usd, latency_ms, client_ip, note, created_at
		   FROM usage_logs%s ORDER BY created_at DESC LIMIT ? OFFSET ?`, cond)

	dataArgs := append(append([]interface{}{}, args...), pageSize, offset)
	rows, err := r.db.QueryContext(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*UsageLog
	for rows.Next() {
		l := &UsageLog{}
		var isStream int
		if err := rows.Scan(
			&l.ID, &l.RequestID, &l.UserID, &l.APIKeyID, &l.Model, &l.ModelAccountID, &l.Protocol,
			&isStream, &l.Status, &l.ErrorMessage,
			&l.InputTokens, &l.OutputTokens, &l.ReasoningTokens,
			&l.CacheCreationTokens, &l.CacheReadTokens, &l.ReasoningEffort,
			&l.TotalTokens,
			&l.CostMicroUSD, &l.LatencyMs, &l.ClientIP, &l.Note, &l.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		l.IsStream = isStream == 1
		list = append(list, l)
	}
	return list, total, rows.Err()
}

// ListPagedWithToken 在 ListPaged 之上 LEFT JOIN api_keys，带回 token 名与分组，
// 专供用户控制台 /me/usage-logs 使用。api_keys 行可能被删除，因此用 LEFT JOIN 容忍空值。
func (r *UsageLogRepo) ListPagedWithToken(ctx context.Context, filter *UsageLogFilter, page, pageSize int) ([]*UsageLog, int64, error) {
	var where []string
	var args []interface{}

	if filter != nil {
		if filter.Model != "" {
			where = append(where, "ul.model = ?")
			args = append(args, filter.Model)
		}
		if filter.Status != "" {
			where = append(where, "ul.status = ?")
			args = append(args, filter.Status)
		}
		if filter.UserID > 0 {
			where = append(where, "ul.user_id = ?")
			args = append(args, filter.UserID)
		}
		if filter.IsStreamFilter != nil {
			if *filter.IsStreamFilter == "true" {
				where = append(where, "ul.is_stream = 1")
			} else if *filter.IsStreamFilter == "false" {
				where = append(where, "ul.is_stream = 0")
			}
		}
		if filter.StartTime != nil {
			where = append(where, "ul.created_at >= ?")
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			where = append(where, "ul.created_at <= ?")
			args = append(args, *filter.EndTime)
		}
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM usage_logs ul" + cond
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	dataQ := fmt.Sprintf(
		`SELECT ul.id, ul.request_id, ul.user_id, ul.api_key_id, ul.model, ul.model_account_id, ul.protocol,
		        ul.is_stream, ul.status, ul.error_message,
		        ul.input_tokens, ul.output_tokens, ul.reasoning_tokens,
		        ul.cache_creation_tokens, ul.cache_read_tokens, ul.reasoning_effort,
		        ul.total_tokens,
		        ul.cost_micro_usd, ul.latency_ms, ul.client_ip, ul.note, ul.created_at,
		        COALESCE(ak.name, ''), COALESCE(ak.token_group, '')
		   FROM usage_logs ul
		   LEFT JOIN api_keys ak ON ak.id = ul.api_key_id
		  %s
		  ORDER BY ul.created_at DESC
		  LIMIT ? OFFSET ?`, cond)

	dataArgs := append(append([]interface{}{}, args...), pageSize, offset)
	rows, err := r.db.QueryContext(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*UsageLog
	for rows.Next() {
		l := &UsageLog{}
		var isStream int
		if err := rows.Scan(
			&l.ID, &l.RequestID, &l.UserID, &l.APIKeyID, &l.Model, &l.ModelAccountID, &l.Protocol,
			&isStream, &l.Status, &l.ErrorMessage,
			&l.InputTokens, &l.OutputTokens, &l.ReasoningTokens,
			&l.CacheCreationTokens, &l.CacheReadTokens, &l.ReasoningEffort,
			&l.TotalTokens,
			&l.CostMicroUSD, &l.LatencyMs, &l.ClientIP, &l.Note, &l.CreatedAt,
			&l.JoinTokenName, &l.JoinTokenGroup,
		); err != nil {
			return nil, 0, err
		}
		l.IsStream = isStream == 1
		list = append(list, l)
	}
	return list, total, rows.Err()
}
