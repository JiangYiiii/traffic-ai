package mysql

import (
	"context"
	"database/sql"

	domain "github.com/trailyai/traffic-ai/internal/domain/routing"
)

type TokenGroupRepo struct {
	db *sql.DB
}

func NewTokenGroupRepo(db *sql.DB) *TokenGroupRepo {
	return &TokenGroupRepo{db: db}
}

func (r *TokenGroupRepo) Create(ctx context.Context, tg *domain.TokenGroup) error {
	const q = `INSERT INTO token_groups (name, description, is_active) VALUES (?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q, tg.Name, tg.Description, tg.IsActive)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	tg.ID = id
	return nil
}

func (r *TokenGroupRepo) FindByID(ctx context.Context, id int64) (*domain.TokenGroup, error) {
	const q = `SELECT id, name, description, is_active, created_at, updated_at
		FROM token_groups WHERE id = ?`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanTokenGroupRow(row)
}

func (r *TokenGroupRepo) FindByName(ctx context.Context, name string) (*domain.TokenGroup, error) {
	const q = `SELECT id, name, description, is_active, created_at, updated_at
		FROM token_groups WHERE name = ?`
	row := r.db.QueryRowContext(ctx, q, name)
	return scanTokenGroupRow(row)
}

func (r *TokenGroupRepo) List(ctx context.Context) ([]*domain.TokenGroup, error) {
	const q = `SELECT id, name, description, is_active, created_at, updated_at
		FROM token_groups ORDER BY id DESC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.TokenGroup
	for rows.Next() {
		tg, err := scanTokenGroupFromRow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, tg)
	}
	return list, rows.Err()
}

func (r *TokenGroupRepo) Update(ctx context.Context, tg *domain.TokenGroup) error {
	const q = `UPDATE token_groups SET name=?, description=?, is_active=? WHERE id=?`
	_, err := r.db.ExecContext(ctx, q, tg.Name, tg.Description, tg.IsActive, tg.ID)
	return err
}

func (r *TokenGroupRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM token_groups WHERE id=?`, id)
	return err
}

func (r *TokenGroupRepo) AddModelAccount(ctx context.Context, tokenGroupID, modelAccountID int64) error {
	const q = `INSERT INTO token_group_model_accounts (token_group_id, model_account_id) VALUES (?, ?)`
	_, err := r.db.ExecContext(ctx, q, tokenGroupID, modelAccountID)
	return err
}

func (r *TokenGroupRepo) RemoveModelAccount(ctx context.Context, tokenGroupID, modelAccountID int64) error {
	const q = `DELETE FROM token_group_model_accounts WHERE token_group_id=? AND model_account_id=?`
	_, err := r.db.ExecContext(ctx, q, tokenGroupID, modelAccountID)
	return err
}

func (r *TokenGroupRepo) ListModelAccountIDs(ctx context.Context, tokenGroupID int64) ([]int64, error) {
	const q = `SELECT model_account_id FROM token_group_model_accounts WHERE token_group_id=?`
	rows, err := r.db.QueryContext(ctx, q, tokenGroupID)
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

func (r *TokenGroupRepo) ListModelAccountIDsByName(ctx context.Context, groupName string) ([]int64, error) {
	const q = `SELECT tgma.model_account_id
		FROM token_group_model_accounts tgma
		JOIN token_groups tg ON tg.id = tgma.token_group_id
		WHERE tg.name = ? AND tg.is_active = 1`
	rows, err := r.db.QueryContext(ctx, q, groupName)
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

func scanTokenGroupFromRow(s scanner) (*domain.TokenGroup, error) {
	var tg domain.TokenGroup
	var isActive int
	err := s.Scan(&tg.ID, &tg.Name, &tg.Description, &isActive, &tg.CreatedAt, &tg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	tg.IsActive = isActive == 1
	return &tg, nil
}

func scanTokenGroupRow(row *sql.Row) (*domain.TokenGroup, error) {
	tg, err := scanTokenGroupFromRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tg, err
}
