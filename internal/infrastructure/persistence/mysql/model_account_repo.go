package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/model"
)

// ModelAccountRepo 访问 model_accounts 表：一个模型下的账号即一条完整的连接方式。
type ModelAccountRepo struct {
	db *sql.DB
}

func NewModelAccountRepo(db *sql.DB) *ModelAccountRepo {
	return &ModelAccountRepo{db: db}
}

const modelAccountColumns = `id, model_id, provider, name, endpoint, credential, auth_type, refresh_token, token_expires_at,
		protocol, weight, is_active, timeout_sec, last_test_ok, last_test_at, last_test_latency_ms, last_test_error, created_at, updated_at`

func (r *ModelAccountRepo) Create(ctx context.Context, a *domain.ModelAccount) error {
	const q = `INSERT INTO model_accounts (model_id, provider, name, endpoint, credential, auth_type, refresh_token, token_expires_at, protocol, weight, is_active, timeout_sec)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		a.ModelID, a.Provider, a.Name, a.Endpoint, a.Credential,
		a.AuthType, nullStr(a.RefreshToken), nullTime(a.TokenExpiresAt),
		a.Protocol, a.Weight, a.IsActive, a.TimeoutSec,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = id
	return nil
}

func (r *ModelAccountRepo) FindByID(ctx context.Context, id int64) (*domain.ModelAccount, error) {
	q := fmt.Sprintf(`SELECT %s FROM model_accounts WHERE id = ?`, modelAccountColumns)
	row := r.db.QueryRowContext(ctx, q, id)
	return scanModelAccountRow(row)
}

func (r *ModelAccountRepo) ListByModelID(ctx context.Context, modelID int64) ([]*domain.ModelAccount, error) {
	q := fmt.Sprintf(`SELECT %s FROM model_accounts WHERE model_id = ? ORDER BY id`, modelAccountColumns)
	rows, err := r.db.QueryContext(ctx, q, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModelAccounts(rows)
}

func (r *ModelAccountRepo) Update(ctx context.Context, a *domain.ModelAccount) error {
	const q = `UPDATE model_accounts SET provider=?, name=?, endpoint=?, credential=?, auth_type=?, refresh_token=?, token_expires_at=?,
		protocol=?, weight=?, is_active=?, timeout_sec=? WHERE id=?`
	_, err := r.db.ExecContext(ctx, q,
		a.Provider, a.Name, a.Endpoint, a.Credential, a.AuthType, nullStr(a.RefreshToken), nullTime(a.TokenExpiresAt),
		a.Protocol, a.Weight, a.IsActive, a.TimeoutSec, a.ID,
	)
	return err
}

func (r *ModelAccountRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM model_accounts WHERE id=?`, id)
	return err
}

func (r *ModelAccountRepo) UpdateLastTest(ctx context.Context, accountID int64, success bool, latencyMs int, errMsg string) error {
	ok := 0
	if success {
		ok = 1
		errMsg = ""
	}
	_, err := r.db.ExecContext(ctx, `UPDATE model_accounts SET last_test_ok=?, last_test_at=?, last_test_latency_ms=?, last_test_error=? WHERE id=?`,
		ok, time.Now().UTC(), latencyMs, errMsg, accountID)
	return err
}

func (r *ModelAccountRepo) ListActiveByModelIDs(ctx context.Context, modelIDs []int64) ([]*domain.ModelAccount, error) {
	if len(modelIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(modelIDs))
	args := make([]interface{}, len(modelIDs))
	for i, id := range modelIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`SELECT %s FROM model_accounts WHERE model_id IN (%s) AND is_active=1 ORDER BY id`,
		modelAccountColumns, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModelAccounts(rows)
}

func (r *ModelAccountRepo) ListByIDs(ctx context.Context, ids []int64) ([]*domain.ModelAccount, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`SELECT %s FROM model_accounts WHERE id IN (%s) ORDER BY id`,
		modelAccountColumns, strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModelAccounts(rows)
}

func (r *ModelAccountRepo) List(ctx context.Context, filter domain.ModelAccountListFilter) ([]*domain.ModelAccount, error) {
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(modelAccountColumns)
	b.WriteString(" FROM model_accounts WHERE 1=1")
	args := make([]any, 0, 3)
	if filter.ModelID > 0 {
		b.WriteString(" AND model_id = ?")
		args = append(args, filter.ModelID)
	}
	if filter.Provider != "" {
		b.WriteString(" AND provider = ?")
		args = append(args, filter.Provider)
	}
	if filter.NameLike != "" {
		b.WriteString(" AND name LIKE ?")
		args = append(args, "%"+filter.NameLike+"%")
	}
	b.WriteString(" ORDER BY id DESC")
	rows, err := r.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModelAccounts(rows)
}

func scanModelAccountFrom(s scanner) (*domain.ModelAccount, error) {
	var a domain.ModelAccount
	var isActive int
	var refreshToken sql.NullString
	var tokenExpiresAt sql.NullTime
	var lastOK sql.NullBool
	var lastAt sql.NullTime
	var lastLat sql.NullInt32
	var lastErr sql.NullString
	err := s.Scan(
		&a.ID, &a.ModelID, &a.Provider, &a.Name, &a.Endpoint, &a.Credential,
		&a.AuthType, &refreshToken, &tokenExpiresAt,
		&a.Protocol, &a.Weight, &isActive, &a.TimeoutSec,
		&lastOK, &lastAt, &lastLat, &lastErr,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.IsActive = isActive == 1
	if refreshToken.Valid {
		a.RefreshToken = refreshToken.String
	}
	if tokenExpiresAt.Valid {
		a.TokenExpiresAt = &tokenExpiresAt.Time
	}
	if lastOK.Valid {
		v := lastOK.Bool == true
		a.LastTestPassed = &v
	}
	if lastAt.Valid {
		t := lastAt.Time
		a.LastTestAt = &t
	}
	if lastLat.Valid {
		v := int(lastLat.Int32)
		a.LastTestLatencyMs = &v
	}
	if lastErr.Valid {
		a.LastTestError = lastErr.String
	}
	return &a, nil
}

func scanModelAccountRow(row *sql.Row) (*domain.ModelAccount, error) {
	a, err := scanModelAccountFrom(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func scanModelAccounts(rows *sql.Rows) ([]*domain.ModelAccount, error) {
	var list []*domain.ModelAccount
	for rows.Next() {
		a, err := scanModelAccountFrom(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
