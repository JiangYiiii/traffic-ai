package mysql

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/trailyai/traffic-ai/internal/domain/routing"
)

type AutoRouteRepo struct {
	db *sql.DB
}

func NewAutoRouteRepo(db *sql.DB) *AutoRouteRepo {
	return &AutoRouteRepo{db: db}
}

func (r *AutoRouteRepo) CreatePolicy(ctx context.Context, p *domain.AutoRoutePolicy) error {
	const q = `INSERT INTO auto_route_policies (virtual_model_id, name, strategy, rules_json, is_active, version)
		VALUES (?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q, p.VirtualModelID, p.Name, p.Strategy, nullableString(p.RulesJSON), p.IsActive, p.Version)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = id
	return nil
}

func (r *AutoRouteRepo) FindPolicyByID(ctx context.Context, id int64) (*domain.AutoRoutePolicy, error) {
	const q = `SELECT id, virtual_model_id, name, strategy, rules_json, is_active, version, created_at, updated_at
		FROM auto_route_policies WHERE id = ?`
	return scanAutoRoutePolicy(r.db.QueryRowContext(ctx, q, id))
}

func (r *AutoRouteRepo) FindActivePolicyByVirtualModelID(ctx context.Context, virtualModelID int64) (*domain.AutoRoutePolicy, error) {
	const q = `SELECT id, virtual_model_id, name, strategy, rules_json, is_active, version, created_at, updated_at
		FROM auto_route_policies WHERE virtual_model_id = ? AND is_active = 1`
	return scanAutoRoutePolicy(r.db.QueryRowContext(ctx, q, virtualModelID))
}

func (r *AutoRouteRepo) ListPolicies(ctx context.Context) ([]*domain.AutoRoutePolicy, error) {
	const q = `SELECT id, virtual_model_id, name, strategy, rules_json, is_active, version, created_at, updated_at
		FROM auto_route_policies ORDER BY id DESC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.AutoRoutePolicy
	for rows.Next() {
		p, err := scanAutoRoutePolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *AutoRouteRepo) UpdatePolicy(ctx context.Context, p *domain.AutoRoutePolicy) error {
	if p.Version <= 0 {
		return fmt.Errorf("auto route policy version must be positive")
	}
	const q = `UPDATE auto_route_policies
		SET virtual_model_id=?, name=?, strategy=?, rules_json=?, is_active=?, version=version+1
		WHERE id=? AND version=?`
	res, err := r.db.ExecContext(ctx, q, p.VirtualModelID, p.Name, p.Strategy, nullableString(p.RulesJSON), p.IsActive, p.ID, p.Version-1)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *AutoRouteRepo) DeletePolicy(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM auto_route_policies WHERE id=?`, id)
	return err
}

func (r *AutoRouteRepo) CreateCandidate(ctx context.Context, c *domain.AutoRouteCandidate) error {
	const q = `INSERT INTO auto_route_candidates
		(policy_id, target_model_id, priority, weight, min_request_context_tokens, quality_score, cost_bias, latency_bias, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		c.PolicyID, c.TargetModelID, c.Priority, c.Weight, c.MinRequestContextTokens,
		c.QualityScore, c.CostBias, c.LatencyBias, c.IsActive,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

func (r *AutoRouteRepo) FindCandidateByID(ctx context.Context, id int64) (*domain.AutoRouteCandidate, error) {
	const q = `SELECT id, policy_id, target_model_id, priority, weight, min_request_context_tokens,
		quality_score, cost_bias, latency_bias, is_active, created_at, updated_at
		FROM auto_route_candidates WHERE id = ?`
	return scanAutoRouteCandidate(r.db.QueryRowContext(ctx, q, id))
}

func (r *AutoRouteRepo) ListCandidatesByPolicyID(ctx context.Context, policyID int64, activeOnly bool) ([]*domain.AutoRouteCandidate, error) {
	q := `SELECT id, policy_id, target_model_id, priority, weight, min_request_context_tokens,
		quality_score, cost_bias, latency_bias, is_active, created_at, updated_at
		FROM auto_route_candidates WHERE policy_id = ?`
	if activeOnly {
		q += ` AND is_active = 1`
	}
	q += ` ORDER BY priority ASC, id ASC`
	rows, err := r.db.QueryContext(ctx, q, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.AutoRouteCandidate
	for rows.Next() {
		c, err := scanAutoRouteCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *AutoRouteRepo) UpdateCandidate(ctx context.Context, c *domain.AutoRouteCandidate) error {
	const q = `UPDATE auto_route_candidates
		SET policy_id=?, target_model_id=?, priority=?, weight=?, min_request_context_tokens=?,
		    quality_score=?, cost_bias=?, latency_bias=?, is_active=?
		WHERE id=?`
	_, err := r.db.ExecContext(ctx, q,
		c.PolicyID, c.TargetModelID, c.Priority, c.Weight, c.MinRequestContextTokens,
		c.QualityScore, c.CostBias, c.LatencyBias, c.IsActive, c.ID,
	)
	return err
}

func (r *AutoRouteRepo) DeleteCandidate(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM auto_route_candidates WHERE id=?`, id)
	return err
}

func scanAutoRoutePolicy(s scanner) (*domain.AutoRoutePolicy, error) {
	var p domain.AutoRoutePolicy
	var rules sql.NullString
	var active int
	err := s.Scan(&p.ID, &p.VirtualModelID, &p.Name, &p.Strategy, &rules, &active, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.RulesJSON = rules.String
	p.IsActive = active == 1
	return &p, nil
}

func scanAutoRouteCandidate(s scanner) (*domain.AutoRouteCandidate, error) {
	var c domain.AutoRouteCandidate
	var active int
	err := s.Scan(
		&c.ID, &c.PolicyID, &c.TargetModelID, &c.Priority, &c.Weight, &c.MinRequestContextTokens,
		&c.QualityScore, &c.CostBias, &c.LatencyBias, &active, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IsActive = active == 1
	return &c, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
