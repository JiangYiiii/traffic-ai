package mysql

import (
	"context"
	"database/sql"

	domain "github.com/trailyai/traffic-ai/internal/domain/ratelimit"
)

type RateLimitRuleRepo struct {
	db *sql.DB
}

func NewRateLimitRuleRepo(db *sql.DB) *RateLimitRuleRepo {
	return &RateLimitRuleRepo{db: db}
}

func (r *RateLimitRuleRepo) Create(ctx context.Context, rule *domain.RateLimitRule) error {
	const q = `INSERT INTO rate_limit_rules
		(name, scope, scope_value, max_rpm, max_tpm, max_concurrent, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		rule.Name, rule.Scope, rule.ScopeValue,
		rule.MaxRPM, rule.MaxTPM, rule.MaxConcurrent, rule.IsActive,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	rule.ID = id
	return nil
}

func (r *RateLimitRuleRepo) Update(ctx context.Context, rule *domain.RateLimitRule) error {
	const q = `UPDATE rate_limit_rules SET
		name=?, scope=?, scope_value=?, max_rpm=?, max_tpm=?,
		max_concurrent=?, is_active=?
		WHERE id=?`
	_, err := r.db.ExecContext(ctx, q,
		rule.Name, rule.Scope, rule.ScopeValue,
		rule.MaxRPM, rule.MaxTPM, rule.MaxConcurrent,
		rule.IsActive, rule.ID,
	)
	return err
}

func (r *RateLimitRuleRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM rate_limit_rules WHERE id=?`, id)
	return err
}

func (r *RateLimitRuleRepo) FindByID(ctx context.Context, id int64) (*domain.RateLimitRule, error) {
	const q = `SELECT id, name, scope, scope_value, max_rpm, max_tpm,
		max_concurrent, is_active, created_at, updated_at
		FROM rate_limit_rules WHERE id=?`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanRateLimitRuleRow(row)
}

func (r *RateLimitRuleRepo) ListAll(ctx context.Context) ([]*domain.RateLimitRule, error) {
	const q = `SELECT id, name, scope, scope_value, max_rpm, max_tpm,
		max_concurrent, is_active, created_at, updated_at
		FROM rate_limit_rules ORDER BY id DESC`
	return r.queryRules(ctx, q)
}

func (r *RateLimitRuleRepo) ListActive(ctx context.Context) ([]*domain.RateLimitRule, error) {
	const q = `SELECT id, name, scope, scope_value, max_rpm, max_tpm,
		max_concurrent, is_active, created_at, updated_at
		FROM rate_limit_rules WHERE is_active=1 ORDER BY id DESC`
	return r.queryRules(ctx, q)
}

func (r *RateLimitRuleRepo) queryRules(ctx context.Context, query string, args ...interface{}) ([]*domain.RateLimitRule, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*domain.RateLimitRule
	for rows.Next() {
		rule, err := scanRateLimitRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func scanRateLimitRuleFrom(s scanner) (*domain.RateLimitRule, error) {
	var rule domain.RateLimitRule
	var isActive int
	err := s.Scan(
		&rule.ID, &rule.Name, &rule.Scope, &rule.ScopeValue,
		&rule.MaxRPM, &rule.MaxTPM, &rule.MaxConcurrent,
		&isActive, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	rule.IsActive = isActive == 1
	return &rule, nil
}

func scanRateLimitRule(rows *sql.Rows) (*domain.RateLimitRule, error) {
	return scanRateLimitRuleFrom(rows)
}

func scanRateLimitRuleRow(row *sql.Row) (*domain.RateLimitRule, error) {
	rule, err := scanRateLimitRuleFrom(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return rule, err
}
