package mysql

import (
	"context"
	"database/sql"

	domain "github.com/trailyai/traffic-ai/internal/domain/billing"
)

type BalanceRepo struct {
	db *sql.DB
}

func NewBalanceRepo(db *sql.DB) *BalanceRepo {
	return &BalanceRepo{db: db}
}

func (r *BalanceRepo) Init(ctx context.Context, userID int64) error {
	const q = `INSERT IGNORE INTO user_balances (user_id, balance) VALUES (?, 0)`
	_, err := r.db.ExecContext(ctx, q, userID)
	return err
}

func (r *BalanceRepo) GetByUserID(ctx context.Context, userID int64) (*domain.Balance, error) {
	const q = `SELECT user_id, balance, total_charged, total_consumed,
		alert_enabled, alert_threshold, updated_at
		FROM user_balances WHERE user_id = ?`
	row := r.db.QueryRowContext(ctx, q, userID)

	var b domain.Balance
	var alertEnabled int
	err := row.Scan(&b.UserID, &b.Balance, &b.TotalCharged, &b.TotalConsumed,
		&alertEnabled, &b.AlertThreshold, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.AlertEnabled = alertEnabled == 1
	return &b, nil
}

// AddBalance 原子增加余额并更新统计列，返回更新后快照。amount 可为负值。
func (r *BalanceRepo) AddBalance(ctx context.Context, userID, amount int64) (*domain.Balance, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if amount >= 0 {
		const q = `UPDATE user_balances SET balance = balance + ?, total_charged = total_charged + ? WHERE user_id = ?`
		if _, err := tx.ExecContext(ctx, q, amount, amount, userID); err != nil {
			return nil, err
		}
	} else {
		const q = `UPDATE user_balances SET balance = balance + ?, total_consumed = total_consumed + ? WHERE user_id = ?`
		if _, err := tx.ExecContext(ctx, q, amount, -amount, userID); err != nil {
			return nil, err
		}
	}

	const sel = `SELECT user_id, balance, total_charged, total_consumed,
		alert_enabled, alert_threshold, updated_at
		FROM user_balances WHERE user_id = ?`
	row := tx.QueryRowContext(ctx, sel, userID)
	var b domain.Balance
	var alertEnabled int
	if err := row.Scan(&b.UserID, &b.Balance, &b.TotalCharged, &b.TotalConsumed,
		&alertEnabled, &b.AlertThreshold, &b.UpdatedAt); err != nil {
		return nil, err
	}
	b.AlertEnabled = alertEnabled == 1

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *BalanceRepo) UpdateAlert(ctx context.Context, userID int64, enabled bool, threshold int64) error {
	const q = `UPDATE user_balances SET alert_enabled = ?, alert_threshold = ? WHERE user_id = ?`
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := r.db.ExecContext(ctx, q, enabledInt, threshold, userID)
	return err
}

// --- BalanceLogRepo ---

type BalanceLogRepo struct {
	db *sql.DB
}

func NewBalanceLogRepo(db *sql.DB) *BalanceLogRepo {
	return &BalanceLogRepo{db: db}
}

func (r *BalanceLogRepo) Create(ctx context.Context, log *domain.BalanceLog) error {
	const q = `INSERT INTO balance_logs
		(user_id, amount, balance_before, balance_after, reason_type, reason_detail, request_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		log.UserID, log.Amount, log.BalanceBefore, log.BalanceAfter,
		log.ReasonType, log.ReasonDetail, log.RequestID)
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

func (r *BalanceLogRepo) ListAllPaged(ctx context.Context, reasonType string, page, pageSize int) ([]*domain.BalanceLog, int64, error) {
	var total int64
	countQ := `SELECT COUNT(*) FROM balance_logs WHERE 1=1`
	args := []interface{}{}
	if reasonType != "" {
		countQ += ` AND reason_type = ?`
		args = append(args, reasonType)
	}
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	dataQ := `SELECT id, user_id, amount, balance_before, balance_after,
		reason_type, reason_detail, request_id, created_at
		FROM balance_logs WHERE 1=1`
	if reasonType != "" {
		dataQ += ` AND reason_type = ?`
	}
	dataQ += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	queryArgs := append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, dataQ, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*domain.BalanceLog
	for rows.Next() {
		var l domain.BalanceLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Amount, &l.BalanceBefore, &l.BalanceAfter,
			&l.ReasonType, &l.ReasonDetail, &l.RequestID, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, &l)
	}
	return logs, total, rows.Err()
}

func (r *BalanceLogRepo) ListByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*domain.BalanceLog, int64, error) {
	var total int64
	const countQ = `SELECT COUNT(*) FROM balance_logs WHERE user_id = ?`
	if err := r.db.QueryRowContext(ctx, countQ, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	const q = `SELECT id, user_id, amount, balance_before, balance_after,
		reason_type, reason_detail, request_id, created_at
		FROM balance_logs WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, q, userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*domain.BalanceLog
	for rows.Next() {
		var l domain.BalanceLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Amount, &l.BalanceBefore, &l.BalanceAfter,
			&l.ReasonType, &l.ReasonDetail, &l.RequestID, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, &l)
	}
	return logs, total, rows.Err()
}
