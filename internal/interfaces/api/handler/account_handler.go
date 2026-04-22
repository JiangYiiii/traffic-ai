package handler

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/trailyai/traffic-ai/internal/domain/auth"
	billingDomain "github.com/trailyai/traffic-ai/internal/domain/billing"
	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/dto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type AccountHandler struct {
	userRepo     auth.UserRepository
	balanceRepo  billingDomain.BalanceRepository
	tokenRepo    *mysql.TokenRepo
	usageLogRepo *mysql.UsageLogRepo
}

func NewAccountHandler(
	userRepo auth.UserRepository,
	balanceRepo billingDomain.BalanceRepository,
	tokenRepo *mysql.TokenRepo,
	usageLogRepo *mysql.UsageLogRepo,
) *AccountHandler {
	return &AccountHandler{
		userRepo:     userRepo,
		balanceRepo:  balanceRepo,
		tokenRepo:    tokenRepo,
		usageLogRepo: usageLogRepo,
	}
}

func (h *AccountHandler) Register(group *gin.RouterGroup) {
	group.GET("/profile", h.profile)
}

func (h *AccountHandler) profile(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()

	user, balance, err := h.loadProfile(ctx, uid)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}

	activeTokenCount := int64(0)
	if h.tokenRepo != nil {
		tokens, tokenErr := h.tokenRepo.ListByUserID(ctx, uid)
		if tokenErr == nil {
			for _, tok := range tokens {
				if tok.IsActive {
					activeTokenCount++
				}
			}
		}
	}

	totalCalls := int64(0)
	if h.usageLogRepo != nil {
		filter := &mysql.UsageLogFilter{UserID: uid}
		_, cnt, logErr := h.usageLogRepo.ListPaged(ctx, filter, 1, 1)
		if logErr == nil {
			totalCalls = cnt
		}
	}

	response.OK(c, dto.ToProfileRespWithDashboard(user, balance, activeTokenCount, totalCalls))
}

func (h *AccountHandler) loadProfile(ctx context.Context, uid int64) (*auth.User, *billingDomain.Balance, error) {
	user, err := h.userRepo.FindByID(ctx, uid)
	if err != nil {
		return nil, nil, errcode.ErrInternal
	}
	if user == nil {
		return nil, nil, errcode.ErrNotFound
	}

	balance, err := h.balanceRepo.GetByUserID(ctx, uid)
	if err != nil {
		return nil, nil, errcode.ErrInternal
	}
	if balance == nil {
		balance = &billingDomain.Balance{UserID: uid}
	}

	return user, balance, nil
}
