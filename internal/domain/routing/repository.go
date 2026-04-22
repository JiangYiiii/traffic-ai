package routing

import "context"

type TokenGroupRepository interface {
	Create(ctx context.Context, tg *TokenGroup) error
	FindByID(ctx context.Context, id int64) (*TokenGroup, error)
	FindByName(ctx context.Context, name string) (*TokenGroup, error)
	List(ctx context.Context) ([]*TokenGroup, error)
	Update(ctx context.Context, tg *TokenGroup) error
	Delete(ctx context.Context, id int64) error

	AddModelAccount(ctx context.Context, tokenGroupID, modelAccountID int64) error
	RemoveModelAccount(ctx context.Context, tokenGroupID, modelAccountID int64) error
	ListModelAccountIDs(ctx context.Context, tokenGroupID int64) ([]int64, error)

	// ListModelAccountIDsByName returns model account IDs bound to the given token group name.
	ListModelAccountIDsByName(ctx context.Context, groupName string) ([]int64, error)
}
