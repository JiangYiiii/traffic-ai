package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/model"
)

type ModelRepo struct {
	db *sql.DB
}

func NewModelRepo(db *sql.DB) *ModelRepo {
	return &ModelRepo{db: db}
}

func (r *ModelRepo) Create(ctx context.Context, m *domain.Model) error {
	const q = `INSERT INTO models (model_name, provider, model_type, billing_type,
		input_price, output_price, reasoning_price, per_request_price, is_active, is_listed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		m.ModelName, m.Provider, m.ModelType, m.BillingType,
		m.InputPrice, m.OutputPrice, m.ReasoningPrice, m.PerRequestPrice, m.IsActive, m.IsListed,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	m.ID = id
	return nil
}

func (r *ModelRepo) FindByID(ctx context.Context, id int64) (*domain.Model, error) {
	const q = `SELECT id, model_name, provider, model_type, billing_type,
		input_price, output_price, reasoning_price, per_request_price,
		is_active, is_listed, last_test_ok, last_test_at, last_test_latency_ms, last_test_error,
		created_at, updated_at
		FROM models WHERE id = ?`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanModelRow(row)
}

func (r *ModelRepo) FindByName(ctx context.Context, name string) (*domain.Model, error) {
	const q = `SELECT id, model_name, provider, model_type, billing_type,
		input_price, output_price, reasoning_price, per_request_price,
		is_active, is_listed, last_test_ok, last_test_at, last_test_latency_ms, last_test_error,
		created_at, updated_at
		FROM models WHERE model_name = ?`
	row := r.db.QueryRowContext(ctx, q, name)
	return scanModelRow(row)
}

func (r *ModelRepo) List(ctx context.Context, filter domain.ListFilter) ([]*domain.Model, error) {
	var b strings.Builder
	b.WriteString(`SELECT id, model_name, provider, model_type, billing_type,
		input_price, output_price, reasoning_price, per_request_price,
		is_active, is_listed, last_test_ok, last_test_at, last_test_latency_ms, last_test_error,
		created_at, updated_at
		FROM models WHERE 1=1`)
	args := make([]any, 0, 2)
	if filter.Provider != "" {
		b.WriteString(` AND provider = ?`)
		args = append(args, filter.Provider)
	}
	if filter.NameLike != "" {
		b.WriteString(` AND model_name LIKE ?`)
		args = append(args, "%"+filter.NameLike+"%")
	}
	b.WriteString(` ORDER BY id DESC`)
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []*domain.Model
	for rows.Next() {
		m, err := scanModelFromRow(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func (r *ModelRepo) ListListedModels(ctx context.Context) ([]*domain.Model, error) {
	const q = `SELECT id, model_name, provider, model_type, billing_type,
		input_price, output_price, reasoning_price, per_request_price,
		is_active, is_listed, last_test_ok, last_test_at, last_test_latency_ms, last_test_error,
		created_at, updated_at
		FROM models WHERE is_active = 1 AND is_listed = 1 ORDER BY model_name`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var models []*domain.Model
	for rows.Next() {
		m, err := scanModelFromRow(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func (r *ModelRepo) Update(ctx context.Context, m *domain.Model) error {
	const q = `UPDATE models SET model_name=?, provider=?, model_type=?, billing_type=?,
		input_price=?, output_price=?, reasoning_price=?, per_request_price=?, is_active=?, is_listed=?
		WHERE id=?`
	_, err := r.db.ExecContext(ctx, q,
		m.ModelName, m.Provider, m.ModelType, m.BillingType,
		m.InputPrice, m.OutputPrice, m.ReasoningPrice, m.PerRequestPrice,
		m.IsActive, m.IsListed, m.ID,
	)
	return err
}

func (r *ModelRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM models WHERE id=?`, id)
	return err
}

func (r *ModelRepo) ListByIDs(ctx context.Context, ids []int64) ([]*domain.Model, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`SELECT id, model_name, provider, model_type, billing_type,
		input_price, output_price, reasoning_price, per_request_price,
		is_active, is_listed, last_test_ok, last_test_at, last_test_latency_ms, last_test_error,
		created_at, updated_at
		FROM models WHERE id IN (%s) ORDER BY id`,
		strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var models []*domain.Model
	for rows.Next() {
		m, err := scanModelFromRow(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func (r *ModelRepo) UpdateLastTest(ctx context.Context, modelID int64, success bool, latencyMs int, errMsg string) error {
	ok := 0
	if success {
		ok = 1
		errMsg = ""
	}
	_, err := r.db.ExecContext(ctx, `UPDATE models SET last_test_ok=?, last_test_at=?, last_test_latency_ms=?, last_test_error=? WHERE id=?`,
		ok, time.Now().UTC(), latencyMs, errMsg, modelID)
	return err
}

func scanModelFromRow(s scanner) (*domain.Model, error) {
	var m domain.Model
	var isActive, isListed int
	var lastOK sql.NullBool
	var lastAt sql.NullTime
	var lastLat sql.NullInt32
	var lastErr sql.NullString
	err := s.Scan(
		&m.ID, &m.ModelName, &m.Provider, &m.ModelType, &m.BillingType,
		&m.InputPrice, &m.OutputPrice, &m.ReasoningPrice, &m.PerRequestPrice,
		&isActive, &isListed, &lastOK, &lastAt, &lastLat, &lastErr,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	m.IsActive = isActive == 1
	m.IsListed = isListed == 1
	if lastOK.Valid {
		v := lastOK.Bool
		m.LastTestPassed = &v
	}
	if lastAt.Valid {
		t := lastAt.Time
		m.LastTestAt = &t
	}
	if lastLat.Valid {
		v := int(lastLat.Int32)
		m.LastTestLatencyMs = &v
	}
	if lastErr.Valid {
		m.LastTestError = lastErr.String
	}
	return &m, nil
}

func scanModelRow(row *sql.Row) (*domain.Model, error) {
	m, err := scanModelFromRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}
