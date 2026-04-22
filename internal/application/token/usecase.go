package token

import (
	"context"
	"fmt"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/token"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

type UseCase struct {
	repo domain.Repository
}

func NewUseCase(repo domain.Repository) *UseCase {
	return &UseCase{repo: repo}
}

const (
	keyPrefix    = "sk-"
	randomLen    = 48
	prefixKeep   = 8 // "sk-" + 前 5 位随机字符
)

func (uc *UseCase) Create(ctx context.Context, userID int64, name, group string, expiresAtStr *string) (string, *domain.Token, error) {
	randPart, err := crypto.GenerateRandomString(randomLen)
	if err != nil {
		logger.L.Errorw("generate random string failed", "error", err)
		return "", nil, errcode.ErrInternal
	}
	plainKey := keyPrefix + randPart
	hash := crypto.HashAPIKey(plainKey)

	prefix := plainKey[:prefixKeep]

	if group == "" {
		group = "default"
	}

	tok := &domain.Token{
		UserID:     userID,
		Name:       name,
		KeyHash:    hash,
		KeyPrefix:  prefix,
		TokenGroup: group,
		IsActive:   true,
	}

	if expiresAtStr != nil && *expiresAtStr != "" {
		t, err := time.Parse(time.RFC3339, *expiresAtStr)
		if err != nil {
			return "", nil, errcode.New(400, 20005, fmt.Sprintf("invalid expires_at format: %v", err))
		}
		tok.ExpiresAt = &t
	}

	if err := uc.repo.Create(ctx, tok); err != nil {
		logger.L.Errorw("create token failed", "error", err, "userID", userID)
		return "", nil, errcode.ErrInternal
	}

	return plainKey, tok, nil
}

func (uc *UseCase) List(ctx context.Context, userID int64) ([]*domain.Token, error) {
	tokens, err := uc.repo.ListByUserID(ctx, userID)
	if err != nil {
		logger.L.Errorw("list tokens failed", "error", err, "userID", userID)
		return nil, errcode.ErrInternal
	}
	return tokens, nil
}

func (uc *UseCase) Enable(ctx context.Context, userID, tokenID int64) error {
	return uc.toggleActive(ctx, userID, tokenID, true)
}

func (uc *UseCase) Disable(ctx context.Context, userID, tokenID int64) error {
	return uc.toggleActive(ctx, userID, tokenID, false)
}

func (uc *UseCase) Delete(ctx context.Context, userID, tokenID int64) error {
	tok, err := uc.repo.FindByID(ctx, tokenID)
	if err != nil {
		logger.L.Errorw("find token failed", "error", err, "tokenID", tokenID)
		return errcode.ErrInternal
	}
	if tok == nil {
		return errcode.ErrNotFound
	}
	if tok.UserID != userID {
		return errcode.ErrForbidden
	}
	if err := uc.repo.Delete(ctx, tokenID); err != nil {
		logger.L.Errorw("delete token failed", "error", err, "tokenID", tokenID)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) LookupByHash(ctx context.Context, hash string) (*domain.Token, error) {
	tok, err := uc.repo.FindByKeyHash(ctx, hash)
	if err != nil {
		logger.L.Errorw("lookup token by hash failed", "error", err)
		return nil, errcode.ErrInternal
	}
	if tok == nil {
		return nil, errcode.ErrInvalidAPIKey
	}
	if !tok.IsActive {
		return nil, errcode.ErrAPIKeyDisabled
	}
	if tok.IsExpired() {
		return nil, errcode.ErrAPIKeyExpired
	}
	return tok, nil
}

func (uc *UseCase) toggleActive(ctx context.Context, userID, tokenID int64, active bool) error {
	tok, err := uc.repo.FindByID(ctx, tokenID)
	if err != nil {
		logger.L.Errorw("find token failed", "error", err, "tokenID", tokenID)
		return errcode.ErrInternal
	}
	if tok == nil {
		return errcode.ErrNotFound
	}
	if tok.UserID != userID {
		return errcode.ErrForbidden
	}
	if err := uc.repo.UpdateActive(ctx, tokenID, active); err != nil {
		logger.L.Errorw("update token active failed", "error", err, "tokenID", tokenID)
		return errcode.ErrInternal
	}
	return nil
}
