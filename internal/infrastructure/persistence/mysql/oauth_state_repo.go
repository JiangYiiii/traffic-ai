package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type OAuthStateRepo struct {
	db *sql.DB
}

func NewOAuthStateRepo(db *sql.DB) *OAuthStateRepo {
	return &OAuthStateRepo{db: db}
}

// Create 插入一条 state 记录（5 分钟过期）。
func (r *OAuthStateRepo) Create(ctx context.Context, state, providerID, codeVerifier string, redirectInfo json.RawMessage) error {
	const q = `INSERT INTO oauth_states (state, provider_id, code_verifier, redirect_info, expires_at)
		VALUES (?, ?, ?, ?, ?)`
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err := r.db.ExecContext(ctx, q, state, providerID, codeVerifier, redirectInfo, expiresAt)
	if err != nil {
		return fmt.Errorf("insert oauth_state: %w", err)
	}
	return nil
}

// Consume 查询并删除 state 记录（一次性使用）。过期或不存在时返回 error。
func (r *OAuthStateRepo) Consume(ctx context.Context, state string) (providerID, codeVerifier string, redirectInfo json.RawMessage, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	const sel = `SELECT provider_id, code_verifier, redirect_info, expires_at
		FROM oauth_states WHERE state = ? FOR UPDATE`
	var expiresAt time.Time
	if err := tx.QueryRowContext(ctx, sel, state).Scan(&providerID, &codeVerifier, &redirectInfo, &expiresAt); err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil, fmt.Errorf("state not found")
		}
		return "", "", nil, fmt.Errorf("query oauth_state: %w", err)
	}

	if time.Now().After(expiresAt) {
		// 过期也删除
		tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state) //nolint:errcheck
		tx.Commit()                                                           //nolint:errcheck
		return "", "", nil, fmt.Errorf("state expired")
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state); err != nil {
		return "", "", nil, fmt.Errorf("delete oauth_state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", "", nil, fmt.Errorf("commit: %w", err)
	}
	return providerID, codeVerifier, redirectInfo, nil
}

// CleanExpired 清理已过期的 state 记录，返回删除行数。
func (r *OAuthStateRepo) CleanExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
