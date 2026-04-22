// Package auth 认证领域模型：用户实体与领域接口。
// @ai_doc Auth领域: 管理用户注册/登录/密码重置/JWT令牌生命周期
package auth

import "time"

// @ai_doc User: 认证领域核心实体，对应 users 表
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string // default | admin | super_admin
	Status       int8   // 1=active, 0=frozen
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const (
	RoleDefault    = "default"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "super_admin"

	StatusActive = int8(1)
	StatusFrozen = int8(0)
)

// @ai_doc_rule IsActive: 只有 status=1 的用户允许登录
func (u *User) IsActive() bool {
	return u.Status == StatusActive
}
