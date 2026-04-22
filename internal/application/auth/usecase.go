// Package auth 认证应用层：编排注册/登录/刷新/密码重置流程。
// @ai_doc_flow AuthUseCase: 调用 domain 接口完成业务编排，不包含持久化细节
package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	domain "github.com/trailyai/traffic-ai/internal/domain/auth"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/jwt"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

const (
	purposeRegister = "register"
	purposeReset    = "reset"
)

// BalanceIniter 仅暴露余额初始化能力，避免 auth 依赖完整 BillingService。
type BalanceIniter interface {
	InitBalance(ctx context.Context, userID int64) error
}

type UseCase struct {
	userRepo    domain.UserRepository
	codeStore   domain.VerifyCodeStore
	lockStore   domain.LoginLockStore
	jwtMgr      *jwt.Manager
	balanceInit BalanceIniter
}

func NewUseCase(
	userRepo domain.UserRepository,
	codeStore domain.VerifyCodeStore,
	lockStore domain.LoginLockStore,
	jwtMgr *jwt.Manager,
	balanceInit BalanceIniter,
) *UseCase {
	return &UseCase{
		userRepo:    userRepo,
		codeStore:   codeStore,
		lockStore:   lockStore,
		jwtMgr:      jwtMgr,
		balanceInit: balanceInit,
	}
}

// @ai_doc_flow SendRegisterCode: 生成6位验证码存入 Redis，日志打印(P1不发邮件)
func (uc *UseCase) SendRegisterCode(ctx context.Context, email string) error {
	existing, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return errcode.ErrInternal
	}
	if existing != nil {
		return errcode.ErrEmailExists
	}

	code := generateCode()
	if err := uc.codeStore.Save(ctx, purposeRegister, email, code); err != nil {
		return errcode.ErrInternal
	}
	// @ai_doc P1阶段: 验证码仅日志输出，不真发邮件
	logger.L.Infof("[AUTH] register verify code for %s: %s", email, code)
	return nil
}

// @ai_doc_flow Register: 校验验证码 → 创建用户 → 签发 JWT
func (uc *UseCase) Register(ctx context.Context, email, password, code string) (*jwt.TokenPair, error) {
	ok, err := uc.codeStore.Verify(ctx, purposeRegister, email, code)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if !ok {
		return nil, errcode.ErrInvalidVerifyCode
	}

	existing, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if existing != nil {
		return nil, errcode.ErrEmailExists
	}

	hash, err := crypto.HashPassword(password)
	if err != nil {
		return nil, errcode.ErrInternal
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: hash,
		Role:         domain.RoleDefault,
		Status:       domain.StatusActive,
	}
	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, errcode.ErrInternal
	}

	if uc.balanceInit != nil {
		if err := uc.balanceInit.InitBalance(ctx, user.ID); err != nil {
			logger.L.Errorw("init balance for new user failed", "error", err, "userID", user.ID)
		}
	}

	pair, err := uc.jwtMgr.GeneratePair(user.ID, user.Role)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	return pair, nil
}

// @ai_doc_flow Login: 锁定检查 → 密码校验 → 失败计数/重置 → 签发 JWT
func (uc *UseCase) Login(ctx context.Context, email, password string) (*jwt.TokenPair, error) {
	locked, err := uc.lockStore.IsLocked(ctx, email)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if locked {
		return nil, errcode.ErrAccountLocked
	}

	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if user == nil {
		_, _ = uc.lockStore.IncrFailCount(ctx, email)
		return nil, errcode.ErrInvalidCredentials
	}

	if !user.IsActive() {
		return nil, errcode.ErrAccountLocked
	}

	if !crypto.CheckPassword(password, user.PasswordHash) {
		cnt, _ := uc.lockStore.IncrFailCount(ctx, email)
		// @ai_doc_rule 登录锁定: 5次失败后锁定15分钟
		if cnt >= 5 {
			logger.L.Warnf("[AUTH] account locked due to %d failed attempts: %s", cnt, email)
		}
		return nil, errcode.ErrInvalidCredentials
	}

	_ = uc.lockStore.Reset(ctx, email)

	pair, err := uc.jwtMgr.GeneratePair(user.ID, user.Role)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	return pair, nil
}

// @ai_doc_flow RefreshToken: 校验 refreshToken 有效期 → 签发新 pair (P1简化: 不做旧token失效)
func (uc *UseCase) RefreshToken(ctx context.Context, refreshToken string) (*jwt.TokenPair, error) {
	claims, err := uc.jwtMgr.ParseRefresh(refreshToken)
	if err != nil {
		return nil, errcode.ErrInvalidRefresh
	}

	pair, err := uc.jwtMgr.GeneratePair(claims.UserID, claims.Role)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	return pair, nil
}

// @ai_doc_flow SendResetCode: 验证邮箱存在 → 生成验证码 → 日志打印
func (uc *UseCase) SendResetCode(ctx context.Context, email string) error {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return errcode.ErrInternal
	}
	// @ai_doc_edge 安全设计: 邮箱不存在时也返回成功，防止用户枚举
	if user == nil {
		return nil
	}

	code := generateCode()
	if err := uc.codeStore.Save(ctx, purposeReset, email, code); err != nil {
		return errcode.ErrInternal
	}
	logger.L.Infof("[AUTH] reset password verify code for %s: %s", email, code)
	return nil
}

// @ai_doc_flow ResetPassword: 校验验证码 → 更新密码
func (uc *UseCase) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	ok, err := uc.codeStore.Verify(ctx, purposeReset, email, code)
	if err != nil {
		return errcode.ErrInternal
	}
	if !ok {
		return errcode.ErrInvalidVerifyCode
	}

	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil || user == nil {
		return errcode.ErrInternal
	}

	hash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return errcode.ErrInternal
	}

	if err := uc.userRepo.UpdatePassword(ctx, user.ID, hash); err != nil {
		return errcode.ErrInternal
	}
	return nil
}

func generateCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}
