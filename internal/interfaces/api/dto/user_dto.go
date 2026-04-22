package dto

import (
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/auth"
)

type AdminUserItem struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

func ToAdminUserItem(u *domain.User) AdminUserItem {
	return AdminUserItem{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		IsActive:  u.IsActive(),
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
	}
}

func ToAdminUserList(users []*domain.User) []AdminUserItem {
	items := make([]AdminUserItem, 0, len(users))
	for _, u := range users {
		items = append(items, ToAdminUserItem(u))
	}
	return items
}
