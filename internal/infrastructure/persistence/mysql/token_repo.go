package mysql

import (
	"context"
	"database/sql"

	domain "github.com/trailyai/traffic-ai/internal/domain/token"
)

type TokenRepo struct {
	db *sql.DB
}

func NewTokenRepo(db *sql.DB) *TokenRepo {
	return &TokenRepo{db: db}
}

func (r *TokenRepo) Create(ctx context.Context, tok *domain.Token) error {
	keyType := tok.KeyType
	if keyType == "" {
		keyType = domain.KeyTypeStandard
	}
	const q = `INSERT INTO api_keys (user_id, name, key_hash, key_prefix, token_group, key_type, is_active, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		tok.UserID, tok.Name, tok.KeyHash, tok.KeyPrefix,
		tok.TokenGroup, keyType, tok.IsActive, tok.ExpiresAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	tok.ID = id
	return nil
}

func (r *TokenRepo) ListByUserID(ctx context.Context, userID int64) ([]*domain.Token, error) {
	const q = `SELECT id, user_id, name, key_hash, key_prefix, token_group, key_type, is_active,
		expires_at, last_used_at, created_at, updated_at
		FROM api_keys WHERE user_id = ? ORDER BY id DESC`
	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*domain.Token
	for rows.Next() {
		tok, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}
	return tokens, rows.Err()
}

func (r *TokenRepo) FindByID(ctx context.Context, id int64) (*domain.Token, error) {
	const q = `SELECT id, user_id, name, key_hash, key_prefix, token_group, key_type, is_active,
		expires_at, last_used_at, created_at, updated_at
		FROM api_keys WHERE id = ?`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanTokenRow(row)
}

func (r *TokenRepo) FindByKeyHash(ctx context.Context, hash string) (*domain.Token, error) {
	const q = `SELECT id, user_id, name, key_hash, key_prefix, token_group, key_type, is_active,
		expires_at, last_used_at, created_at, updated_at
		FROM api_keys WHERE key_hash = ?`
	row := r.db.QueryRowContext(ctx, q, hash)
	return scanTokenRow(row)
}

func (r *TokenRepo) UpdateActive(ctx context.Context, id int64, active bool) error {
	const q = `UPDATE api_keys SET is_active = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, active, id)
	return err
}

func (r *TokenRepo) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM api_keys WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, id)
	return err
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTokenFromRow(s scanner) (*domain.Token, error) {
	var tok domain.Token
	var isActive int
	err := s.Scan(
		&tok.ID, &tok.UserID, &tok.Name, &tok.KeyHash, &tok.KeyPrefix,
		&tok.TokenGroup, &tok.KeyType, &isActive, &tok.ExpiresAt, &tok.LastUsedAt,
		&tok.CreatedAt, &tok.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	tok.IsActive = isActive == 1
	if tok.KeyType == "" {
		tok.KeyType = domain.KeyTypeStandard
	}
	return &tok, nil
}

func scanToken(rows *sql.Rows) (*domain.Token, error) {
	return scanTokenFromRow(rows)
}

func scanTokenRow(row *sql.Row) (*domain.Token, error) {
	tok, err := scanTokenFromRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tok, err
}
