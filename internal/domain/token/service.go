package token

import "context"

type Service interface {
	Create(ctx context.Context, userID int64, name, group string, expiresAt *string) (plainKey string, tok *Token, err error)
	List(ctx context.Context, userID int64) ([]*Token, error)
	Enable(ctx context.Context, userID, tokenID int64) error
	Disable(ctx context.Context, userID, tokenID int64) error
	Delete(ctx context.Context, userID, tokenID int64) error
	LookupByHash(ctx context.Context, hash string) (*Token, error)
}
