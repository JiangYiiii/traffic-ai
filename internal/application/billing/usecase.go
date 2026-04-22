// Package billing 余额管理应用层，编排 Domain/Infra 完成充值、扣费、兑换、流水等用例。
package billing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/billing"
	rediscache "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

type UseCase struct {
	db             *sql.DB
	balanceRepo    domain.BalanceRepository
	logRepo        domain.BalanceLogRepository
	redeemRepo     domain.RedeemCodeRepository
	balanceCache   *rediscache.BalanceCache
}

func NewUseCase(
	db *sql.DB,
	balanceRepo domain.BalanceRepository,
	logRepo domain.BalanceLogRepository,
	redeemRepo domain.RedeemCodeRepository,
	cache *rediscache.BalanceCache,
) *UseCase {
	return &UseCase{
		db:           db,
		balanceRepo:  balanceRepo,
		logRepo:      logRepo,
		redeemRepo:   redeemRepo,
		balanceCache: cache,
	}
}

// --------------- BillingService 实现 (供网关调用) ---------------

func (uc *UseCase) InitBalance(ctx context.Context, userID int64) error {
	if err := uc.balanceRepo.Init(ctx, userID); err != nil {
		logger.L.Errorw("init balance failed", "error", err, "userID", userID)
		return errcode.ErrInternal
	}
	_ = uc.balanceCache.Set(ctx, userID, 0)
	return nil
}

func (uc *UseCase) CheckBalance(ctx context.Context, userID int64, requiredMicroUSD int64) error {
	balance, err := uc.getBalance(ctx, userID)
	if err != nil {
		return err
	}
	if balance < requiredMicroUSD {
		return errcode.ErrInsufficientBalance
	}
	return nil
}

// PreDeduct Redis 原子预扣费，扣费后异步写 MySQL 流水。
func (uc *UseCase) PreDeduct(ctx context.Context, userID int64, estimatedMicroUSD int64, requestID string) error {
	if err := uc.ensureCache(ctx, userID); err != nil {
		return err
	}

	_, err := uc.balanceCache.DecrBy(ctx, userID, estimatedMicroUSD)
	if err != nil {
		return errcode.ErrInsufficientBalance
	}

	go uc.asyncWriteLog(userID, -estimatedMicroUSD, domain.ReasonConsume, "pre-deduct", requestID)
	return nil
}

// Settle 最终结算，多退少补。
func (uc *UseCase) Settle(ctx context.Context, userID int64, actualMicroUSD int64, preDeducted int64, requestID string, detail string) error {
	diff := preDeducted - actualMicroUSD // >0 多扣需退，<0 欠扣需补
	if diff == 0 {
		return nil
	}

	if diff > 0 {
		if _, err := uc.balanceCache.IncrBy(ctx, userID, diff); err != nil {
			logger.L.Errorw("settle refund cache failed", "error", err, "userID", userID)
		}
		go uc.asyncWriteLog(userID, diff, domain.ReasonRefund, detail, requestID)
	} else {
		if _, err := uc.balanceCache.DecrBy(ctx, userID, -diff); err != nil {
			logger.L.Warnw("settle extra deduct cache failed, will sync from mysql", "error", err, "userID", userID)
		}
		go uc.asyncWriteLog(userID, diff, domain.ReasonConsume, detail, requestID)
	}
	return nil
}

// --------------- 用户控制台用例 ---------------

func (uc *UseCase) GetBalance(ctx context.Context, userID int64) (*domain.Balance, error) {
	b, err := uc.balanceRepo.GetByUserID(ctx, userID)
	if err != nil {
		logger.L.Errorw("get balance failed", "error", err, "userID", userID)
		return nil, errcode.ErrInternal
	}
	if b == nil {
		return nil, errcode.ErrNotFound
	}
	return b, nil
}

func (uc *UseCase) ListLogs(ctx context.Context, userID int64, page, pageSize int) ([]*domain.BalanceLog, int64, error) {
	logs, total, err := uc.logRepo.ListByUserID(ctx, userID, page, pageSize)
	if err != nil {
		logger.L.Errorw("list balance logs failed", "error", err, "userID", userID)
		return nil, 0, errcode.ErrInternal
	}
	return logs, total, nil
}

// Redeem 在同一事务内完成: Claim 兑换码 + Init 余额行 + AddBalance + 写流水，任一步失败整体回滚。
func (uc *UseCase) Redeem(ctx context.Context, userID int64, code string) (*domain.Balance, error) {
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		logger.L.Errorw("begin redeem tx failed", "error", err)
		return nil, errcode.ErrInternal
	}
	defer tx.Rollback()

	// 1. Claim（SELECT ... FOR UPDATE + UPDATE）
	const selQ = `SELECT id, code, amount, status FROM redeem_codes WHERE code = ? FOR UPDATE`
	var rc domain.RedeemCode
	if err := tx.QueryRowContext(ctx, selQ, code).Scan(&rc.ID, &rc.Code, &rc.Amount, &rc.Status); err != nil {
		if err == sql.ErrNoRows {
			return nil, errcode.ErrInvalidRedeemCode
		}
		logger.L.Errorw("claim select failed", "error", err)
		return nil, errcode.ErrInternal
	}
	if rc.Status != domain.RedeemStatusUnused {
		return nil, errcode.ErrRedeemCodeUsed
	}

	now := time.Now()
	const updCodeQ = `UPDATE redeem_codes SET status = ?, used_by = ?, used_at = ? WHERE id = ?`
	if _, err := tx.ExecContext(ctx, updCodeQ, domain.RedeemStatusUsed, userID, now, rc.ID); err != nil {
		logger.L.Errorw("claim update failed", "error", err)
		return nil, errcode.ErrInternal
	}

	// 2. 兜底初始化余额行
	const initQ = `INSERT IGNORE INTO user_balances (user_id, balance) VALUES (?, 0)`
	if _, err := tx.ExecContext(ctx, initQ, userID); err != nil {
		logger.L.Errorw("init balance in redeem tx failed", "error", err)
		return nil, errcode.ErrInternal
	}

	// 3. 读取 before 余额
	var before int64
	const balQ = `SELECT balance FROM user_balances WHERE user_id = ?`
	_ = tx.QueryRowContext(ctx, balQ, userID).Scan(&before)

	// 4. AddBalance
	const addQ = `UPDATE user_balances SET balance = balance + ?, total_charged = total_charged + ? WHERE user_id = ?`
	if _, err := tx.ExecContext(ctx, addQ, rc.Amount, rc.Amount, userID); err != nil {
		logger.L.Errorw("add balance in redeem tx failed", "error", err)
		return nil, errcode.ErrInternal
	}

	// 5. 读取 after 余额
	const selBalQ = `SELECT user_id, balance, total_charged, total_consumed, alert_enabled, alert_threshold, updated_at FROM user_balances WHERE user_id = ?`
	var b domain.Balance
	var alertEnabled int
	if err := tx.QueryRowContext(ctx, selBalQ, userID).Scan(&b.UserID, &b.Balance, &b.TotalCharged, &b.TotalConsumed, &alertEnabled, &b.AlertThreshold, &b.UpdatedAt); err != nil {
		logger.L.Errorw("read balance after add failed", "error", err)
		return nil, errcode.ErrInternal
	}
	b.AlertEnabled = alertEnabled == 1

	// 6. 写流水
	const logQ = `INSERT INTO balance_logs (user_id, amount, balance_before, balance_after, reason_type, reason_detail, request_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, logQ, userID, rc.Amount, before, b.Balance, domain.ReasonRedeem, fmt.Sprintf("redeem:%s", code), ""); err != nil {
		logger.L.Errorw("write balance log in redeem tx failed", "error", err)
		return nil, errcode.ErrInternal
	}

	// 7. 提交
	if err := tx.Commit(); err != nil {
		logger.L.Errorw("commit redeem tx failed", "error", err)
		return nil, errcode.ErrInternal
	}

	_ = uc.balanceCache.Set(ctx, userID, b.Balance)
	return &b, nil
}

func (uc *UseCase) UpdateAlert(ctx context.Context, userID int64, enabled bool, threshold int64) error {
	if err := uc.balanceRepo.UpdateAlert(ctx, userID, enabled, threshold); err != nil {
		logger.L.Errorw("update alert failed", "error", err, "userID", userID)
		return errcode.ErrInternal
	}
	return nil
}

// --------------- 管理员用例 ---------------

func (uc *UseCase) AdminCharge(ctx context.Context, userID, amount int64, detail string) (*domain.Balance, error) {
	b, err := uc.addAndSync(ctx, userID, amount, domain.ReasonCharge, detail, "")
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (uc *UseCase) BatchCreateRedeemCodes(ctx context.Context, count int, amount, createdBy int64) ([]string, error) {
	codes := make([]*domain.RedeemCode, 0, count)
	plainCodes := make([]string, 0, count)
	for range count {
		c, err := generateRedeemCode()
		if err != nil {
			logger.L.Errorw("generate redeem code failed", "error", err)
			return nil, errcode.ErrInternal
		}
		plainCodes = append(plainCodes, c)
		codes = append(codes, &domain.RedeemCode{
			Code:      c,
			Amount:    amount,
			CreatedBy: createdBy,
		})
	}

	if err := uc.redeemRepo.BatchCreate(ctx, codes); err != nil {
		logger.L.Errorw("batch create redeem codes failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return plainCodes, nil
}

func (uc *UseCase) ListAllLogs(ctx context.Context, reasonType string, page, pageSize int) ([]*domain.BalanceLog, int64, error) {
	logs, total, err := uc.logRepo.ListAllPaged(ctx, reasonType, page, pageSize)
	if err != nil {
		logger.L.Errorw("list all balance logs failed", "error", err)
		return nil, 0, errcode.ErrInternal
	}
	return logs, total, nil
}

func (uc *UseCase) ListRedeemCodes(ctx context.Context, page, pageSize int) ([]*domain.RedeemCode, int64, error) {
	list, total, err := uc.redeemRepo.List(ctx, page, pageSize)
	if err != nil {
		logger.L.Errorw("list redeem codes failed", "error", err)
		return nil, 0, errcode.ErrInternal
	}
	return list, total, nil
}

// --------------- 内部辅助 ---------------

func (uc *UseCase) getBalance(ctx context.Context, userID int64) (int64, error) {
	bal, ok, err := uc.balanceCache.Get(ctx, userID)
	if err == nil && ok {
		return bal, nil
	}
	b, err := uc.balanceRepo.GetByUserID(ctx, userID)
	if err != nil {
		logger.L.Errorw("get balance from db failed", "error", err, "userID", userID)
		return 0, errcode.ErrInternal
	}
	if b == nil {
		return 0, errcode.ErrInsufficientBalance
	}
	_ = uc.balanceCache.Set(ctx, userID, b.Balance)
	return b.Balance, nil
}

func (uc *UseCase) ensureCache(ctx context.Context, userID int64) error {
	_, ok, err := uc.balanceCache.Get(ctx, userID)
	if err != nil {
		return errcode.ErrInternal
	}
	if ok {
		return nil
	}
	b, err := uc.balanceRepo.GetByUserID(ctx, userID)
	if err != nil {
		return errcode.ErrInternal
	}
	if b == nil {
		return errcode.ErrInsufficientBalance
	}
	_ = uc.balanceCache.Set(ctx, userID, b.Balance)
	return nil
}

// addAndSync MySQL 加款 → 写流水 → 同步 Redis 缓存。
func (uc *UseCase) addAndSync(ctx context.Context, userID, amount int64, reason, detail, requestID string) (*domain.Balance, error) {
	_ = uc.balanceRepo.Init(ctx, userID)

	before, _ := uc.getBalance(ctx, userID)

	b, err := uc.balanceRepo.AddBalance(ctx, userID, amount)
	if err != nil {
		logger.L.Errorw("add balance failed", "error", err, "userID", userID)
		return nil, errcode.ErrInternal
	}

	_ = uc.balanceCache.Set(ctx, userID, b.Balance)

	if err := uc.logRepo.Create(ctx, &domain.BalanceLog{
		UserID:        userID,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  b.Balance,
		ReasonType:    reason,
		ReasonDetail:  detail,
		RequestID:     requestID,
	}); err != nil {
		logger.L.Errorw("write balance log failed", "error", err, "userID", userID)
	}

	return b, nil
}

// asyncWriteLog 异步写 MySQL 流水（预扣 / 结算场景）。
//
// 这里一定要**先 AddBalance 再写流水**：
//  1. AddBalance 返回值里的 b.Balance 就是本次操作完成后的真实余额 → balanceAfter；
//  2. balanceBefore 通过 balanceAfter - amount 反推，语义上始终与 amount 自洽。
//
// 之前的实现是先 GetByUserID 读一次 MySQL 当做 after、再算 before，这样会把
// “本次尚未扣减的余额” 当成 after，before 还比它更高，导致用户控制台流水看起来完全反了。
func (uc *UseCase) asyncWriteLog(userID, amount int64, reason, detail, requestID string) {
	ctx := context.Background()

	b, err := uc.balanceRepo.AddBalance(ctx, userID, amount)
	if err != nil || b == nil {
		logger.L.Errorw("async apply balance failed", "error", err, "userID", userID)
		return
	}
	balanceAfter := b.Balance
	balanceBefore := balanceAfter - amount

	if err := uc.logRepo.Create(ctx, &domain.BalanceLog{
		UserID:        userID,
		Amount:        amount,
		BalanceBefore: balanceBefore,
		BalanceAfter:  balanceAfter,
		ReasonType:    reason,
		ReasonDetail:  detail,
		RequestID:     requestID,
	}); err != nil {
		logger.L.Errorw("async write balance log failed", "error", err, "userID", userID)
	}
}

func generateRedeemCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
