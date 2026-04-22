package dto

import (
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/token"
)

// ---------- Request ----------

type CreateTokenReq struct {
	Name       string  `json:"name" binding:"required,max=100"`
	TokenGroup string  `json:"token_group"`
	ExpiresAt  *string `json:"expires_at"`
}

// ---------- Response ----------

type CreateTokenResp struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	KeyPrefix string `json:"key_prefix"`
	CreatedAt string `json:"created_at"`
}

type TokenItem struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	KeyDisplay string  `json:"key_display"`
	TokenGroup string  `json:"token_group"`
	IsActive   bool    `json:"is_active"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

func ToTokenItem(tok *domain.Token) TokenItem {
	item := TokenItem{
		ID:         tok.ID,
		Name:       tok.Name,
		KeyDisplay: tok.MaskedKey(),
		TokenGroup: tok.TokenGroup,
		IsActive:   tok.IsActive,
		CreatedAt:  tok.CreatedAt.Format(time.RFC3339),
	}
	if tok.ExpiresAt != nil {
		s := tok.ExpiresAt.Format(time.RFC3339)
		item.ExpiresAt = &s
	}
	if tok.LastUsedAt != nil {
		s := tok.LastUsedAt.Format(time.RFC3339)
		item.LastUsedAt = &s
	}
	return item
}

func ToTokenItemList(tokens []*domain.Token) []TokenItem {
	items := make([]TokenItem, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, ToTokenItem(t))
	}
	return items
}
