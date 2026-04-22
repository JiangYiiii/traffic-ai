package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/billing"
)

type RedeemCodeRepo struct {
	db *sql.DB
}

func NewRedeemCodeRepo(db *sql.DB) *RedeemCodeRepo {
	return &RedeemCodeRepo{db: db}
}

// Claim 原子地占用兑换码，返回兑换码信息；若不存在或已使用则返回 nil。
func (r *RedeemCodeRepo) Claim(ctx context.Context, code string, userID int64) (*domain.RedeemCode, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	const selQ = `SELECT id, code, amount, status FROM redeem_codes WHERE code = ? FOR UPDATE`
	var rc domain.RedeemCode
	if err := tx.QueryRowContext(ctx, selQ, code).Scan(&rc.ID, &rc.Code, &rc.Amount, &rc.Status); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if rc.Status != domain.RedeemStatusUnused {
		return nil, nil
	}

	now := time.Now()
	const updQ = `UPDATE redeem_codes SET status = ?, used_by = ?, used_at = ? WHERE id = ?`
	if _, err := tx.ExecContext(ctx, updQ, domain.RedeemStatusUsed, userID, now, rc.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	rc.Status = domain.RedeemStatusUsed
	rc.UsedBy = &userID
	rc.UsedAt = &now
	return &rc, nil
}

func (r *RedeemCodeRepo) BatchCreate(ctx context.Context, codes []*domain.RedeemCode) error {
	if len(codes) == 0 {
		return nil
	}
	const base = `INSERT INTO redeem_codes (code, amount, created_by) VALUES `
	var b strings.Builder
	b.WriteString(base)
	args := make([]interface{}, 0, len(codes)*3)
	for i, c := range codes {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("(?,?,?)")
		args = append(args, c.Code, c.Amount, c.CreatedBy)
	}
	_, err := r.db.ExecContext(ctx, b.String(), args...)
	return err
}

func (r *RedeemCodeRepo) List(ctx context.Context, page, pageSize int) ([]*domain.RedeemCode, int64, error) {
	var total int64
	const countQ = `SELECT COUNT(*) FROM redeem_codes`
	if err := r.db.QueryRowContext(ctx, countQ).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	const q = `SELECT id, code, amount, status, used_by, used_at, created_by, created_at
		FROM redeem_codes ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, q, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*domain.RedeemCode
	for rows.Next() {
		var rc domain.RedeemCode
		if err := rows.Scan(&rc.ID, &rc.Code, &rc.Amount, &rc.Status,
			&rc.UsedBy, &rc.UsedAt, &rc.CreatedBy, &rc.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, &rc)
	}
	return list, total, rows.Err()
}
