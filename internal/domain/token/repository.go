package token

import "context"

type Repository interface {
	Create(ctx context.Context, tok *Token) error
	ListByUserID(ctx context.Context, userID int64) ([]*Token, error)
	FindByID(ctx context.Context, id int64) (*Token, error)
	FindByKeyHash(ctx context.Context, hash string) (*Token, error)
	UpdateActive(ctx context.Context, id int64, active bool) error
	Delete(ctx context.Context, id int64) error
}
