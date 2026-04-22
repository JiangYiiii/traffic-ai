package token

import "time"

type Token struct {
	ID         int64
	UserID     int64
	Name       string
	KeyHash    string
	KeyPrefix  string
	TokenGroup string
	KeyType    string // standard | openclaw_token
	IsActive   bool
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const (
	KeyTypeStandard     = "standard"
	KeyTypeOpenClawToken = "openclaw_token"
)

func (t *Token) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return t.ExpiresAt.Before(time.Now())
}

func (t *Token) MaskedKey() string {
	return t.KeyPrefix + "****"
}
