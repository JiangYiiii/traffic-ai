package auth

import "context"

// @ai_doc UserRepository: 用户持久化接口，由 infrastructure 层实现
type UserRepository interface {
	FindByID(ctx context.Context, id int64) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, user *User) error
	UpdatePassword(ctx context.Context, userID int64, passwordHash string) error
}

// @ai_doc VerifyCodeStore: 验证码存取接口(Redis 实现)，支持注册和密码重置场景
type VerifyCodeStore interface {
	// @ai_doc_rule 验证码: 6位数字，TTL 5分钟，验证后删除
	Save(ctx context.Context, purpose, email, code string) error
	Verify(ctx context.Context, purpose, email, code string) (bool, error)
}

// @ai_doc LoginLockStore: 登录失败锁定接口，5次失败锁15分钟
type LoginLockStore interface {
	// @ai_doc_rule IncrFailCount: 每次登录失败 +1，首次失败设 15 分钟 TTL 窗口
	IncrFailCount(ctx context.Context, email string) (int64, error)
	IsLocked(ctx context.Context, email string) (bool, error)
	Reset(ctx context.Context, email string) error
}
