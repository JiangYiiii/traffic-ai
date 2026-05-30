package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
)

func TestAutoRouteRepoPolicyLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewAutoRouteRepo(db)
	ctx := context.Background()
	now := time.Now()

	policy := &domainRouting.AutoRoutePolicy{
		VirtualModelID: 12,
		Name:           "Default AUTO",
		Strategy:       domainRouting.AutoStrategyBalanced,
		RulesJSON:      `{"fallback_across_models":true}`,
		IsActive:       true,
		Version:        1,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO auto_route_policies (virtual_model_id, name, strategy, rules_json, is_active, version)
		VALUES (?, ?, ?, ?, ?, ?)`)).
		WithArgs(policy.VirtualModelID, policy.Name, policy.Strategy, policy.RulesJSON, true, policy.Version).
		WillReturnResult(sqlmock.NewResult(101, 1))

	if err := repo.CreatePolicy(ctx, policy); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	if policy.ID != 101 {
		t.Fatalf("policy ID = %d, want 101", policy.ID)
	}

	rows := sqlmock.NewRows([]string{
		"id", "virtual_model_id", "name", "strategy", "rules_json", "is_active", "version", "created_at", "updated_at",
	}).AddRow(int64(101), int64(12), "Default AUTO", domainRouting.AutoStrategyBalanced, `{"fallback_across_models":true}`, 1, 1, now, now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, virtual_model_id, name, strategy, rules_json, is_active, version, created_at, updated_at
		FROM auto_route_policies WHERE virtual_model_id = ? AND is_active = 1`)).
		WithArgs(int64(12)).
		WillReturnRows(rows)

	got, err := repo.FindActivePolicyByVirtualModelID(ctx, 12)
	if err != nil {
		t.Fatalf("FindActivePolicyByVirtualModelID: %v", err)
	}
	if got == nil || got.ID != 101 || !got.IsActive || got.RulesJSON == "" {
		t.Fatalf("unexpected policy: %#v", got)
	}

	policy.Name = "AUTO Updated"
	policy.Version = 2
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE auto_route_policies
		SET virtual_model_id=?, name=?, strategy=?, rules_json=?, is_active=?, version=version+1
		WHERE id=? AND version=?`)).
		WithArgs(policy.VirtualModelID, policy.Name, policy.Strategy, policy.RulesJSON, true, policy.ID, policy.Version-1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdatePolicy(ctx, policy); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM auto_route_policies WHERE id=?`)).
		WithArgs(policy.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.DeletePolicy(ctx, policy.ID); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAutoRouteRepoCandidateLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewAutoRouteRepo(db)
	ctx := context.Background()
	now := time.Now()

	candidate := &domainRouting.AutoRouteCandidate{
		PolicyID:                101,
		TargetModelID:           202,
		Priority:                10,
		Weight:                  3,
		MinRequestContextTokens: 2000,
		QualityScore:            80,
		CostBias:                20,
		LatencyBias:             30,
		IsActive:                true,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO auto_route_candidates
		(policy_id, target_model_id, priority, weight, min_request_context_tokens, quality_score, cost_bias, latency_bias, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)).
		WithArgs(candidate.PolicyID, candidate.TargetModelID, candidate.Priority, candidate.Weight, candidate.MinRequestContextTokens, candidate.QualityScore, candidate.CostBias, candidate.LatencyBias, true).
		WillReturnResult(sqlmock.NewResult(303, 1))

	if err := repo.CreateCandidate(ctx, candidate); err != nil {
		t.Fatalf("CreateCandidate: %v", err)
	}
	if candidate.ID != 303 {
		t.Fatalf("candidate ID = %d, want 303", candidate.ID)
	}

	rows := sqlmock.NewRows([]string{
		"id", "policy_id", "target_model_id", "priority", "weight", "min_request_context_tokens", "quality_score", "cost_bias", "latency_bias", "is_active", "created_at", "updated_at",
	}).AddRow(int64(303), int64(101), int64(202), 10, 3, 2000, 80, 20, 30, 1, now, now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, policy_id, target_model_id, priority, weight, min_request_context_tokens,
		quality_score, cost_bias, latency_bias, is_active, created_at, updated_at
		FROM auto_route_candidates WHERE policy_id = ? AND is_active = 1 ORDER BY priority ASC, id ASC`)).
		WithArgs(int64(101)).
		WillReturnRows(rows)

	list, err := repo.ListCandidatesByPolicyID(ctx, 101, true)
	if err != nil {
		t.Fatalf("ListCandidatesByPolicyID: %v", err)
	}
	if len(list) != 1 || list[0].ID != 303 || list[0].QualityScore != 80 {
		t.Fatalf("unexpected candidates: %#v", list)
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM auto_route_candidates WHERE id=?`)).
		WithArgs(candidate.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.DeleteCandidate(ctx, candidate.ID); err != nil {
		t.Fatalf("DeleteCandidate: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
