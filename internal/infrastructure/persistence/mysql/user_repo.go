package mysql

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/trailyai/traffic-ai/internal/domain/auth"
)

// @ai_doc UserRepo: users 表的 MySQL 持久化实现
type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) FindByID(ctx context.Context, id int64) (*domain.User, error) {
	const q = `SELECT id, email, password_hash, role, status, created_at, updated_at
		FROM users WHERE id = ?`

	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash,
		&u.Role, &u.Status,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	const q = `SELECT id, email, password_hash, role, status, created_at, updated_at
		FROM users WHERE email = ?`

	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, q, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash,
		&u.Role, &u.Status,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) Create(ctx context.Context, user *domain.User) error {
	const q = `INSERT INTO users (email, password_hash, role, status) VALUES (?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, q, user.Email, user.PasswordHash, user.Role, user.Status)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	return nil
}

func (r *UserRepo) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	const q = `UPDATE users SET password_hash = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, passwordHash, userID)
	return err
}

func (r *UserRepo) ListPaged(ctx context.Context, emailLike string, page, pageSize int) ([]*domain.User, int64, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if emailLike != "" {
		where += " AND email LIKE ?"
		args = append(args, "%"+emailLike+"%")
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM users " + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	listQ := "SELECT id, email, role, status, created_at FROM users " + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	listArgs := append(append([]interface{}{}, args...), pageSize, offset)

	rows, err := r.db.QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u := &domain.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}
