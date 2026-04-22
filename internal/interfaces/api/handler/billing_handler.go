package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	appbilling "github.com/trailyai/traffic-ai/internal/application/billing"
	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/dto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type BillingHandler struct {
	uc       *appbilling.UseCase
	userRepo *mysql.UserRepo
}

func NewBillingHandler(useCase *appbilling.UseCase, userRepo *mysql.UserRepo) *BillingHandler {
	return &BillingHandler{uc: useCase, userRepo: userRepo}
}

// RegisterUser 用户控制台路由，挂载在需 JWT 鉴权的 group 下。
func (h *BillingHandler) RegisterUser(group *gin.RouterGroup) {
	group.GET("/balance/logs", h.listLogs)
	group.POST("/balance/redeem", h.redeem)
	group.PATCH("/balance-alert", h.updateAlert)
}

// RegisterAdmin 管理后台路由。
func (h *BillingHandler) RegisterAdmin(group *gin.RouterGroup) {
	group.GET("/users", h.listUsers)
	group.POST("/users/:id/charge", h.adminCharge)
	group.POST("/redeem-codes/batch", h.batchCreateRedeemCodes)
	group.GET("/redeem-codes", h.listRedeemCodes)
	group.GET("/balance-logs", h.adminListBalanceLogs)
}

// ---- 用户控制台 ----

func (h *BillingHandler) listLogs(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	page, pageSize := parsePage(c)
	logs, total, err := h.uc.ListLogs(c.Request.Context(), uid, page, pageSize)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.PageResult(c, dto.ToBalanceLogList(logs), total, page, pageSize)
}

func (h *BillingHandler) redeem(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var req dto.RedeemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	b, err := h.uc.Redeem(c.Request.Context(), uid, req.Code)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OK(c, dto.ToBalanceResp(b))
}

func (h *BillingHandler) updateAlert(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var req dto.UpdateAlertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	if err := h.uc.UpdateAlert(c.Request.Context(), uid, req.Enabled, req.Threshold); err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OKMsg(c, "ok")
}

// ---- 管理后台 ----

func (h *BillingHandler) listUsers(c *gin.Context) {
	page, pageSize := parsePage(c)
	email := c.Query("email")
	users, total, err := h.userRepo.ListPaged(c.Request.Context(), email, page, pageSize)
	if err != nil {
		response.Fail(c, errcode.ErrInternal)
		return
	}
	response.PageResult(c, dto.ToAdminUserList(users), total, page, pageSize)
}

func (h *BillingHandler) adminCharge(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	var req dto.AdminChargeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	b, err := h.uc.AdminCharge(c.Request.Context(), targetID, req.Amount, req.Detail)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OK(c, dto.ToBalanceResp(b))
}

func (h *BillingHandler) batchCreateRedeemCodes(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}

	var req dto.BatchRedeemCodesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	codes, err := h.uc.BatchCreateRedeemCodes(c.Request.Context(), req.Count, req.Amount, uid)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.OK(c, gin.H{"codes": codes, "count": len(codes)})
}

func (h *BillingHandler) listRedeemCodes(c *gin.Context) {
	page, pageSize := parsePage(c)
	codes, total, err := h.uc.ListRedeemCodes(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.PageResult(c, dto.ToRedeemCodeList(codes), total, page, pageSize)
}

func (h *BillingHandler) adminListBalanceLogs(c *gin.Context) {
	page, pageSize := parsePage(c)
	reasonType := c.Query("reason_type")
	logs, total, err := h.uc.ListAllLogs(c.Request.Context(), reasonType, page, pageSize)
	if err != nil {
		response.Fail(c, toAppErr(err))
		return
	}
	response.PageResult(c, dto.ToBalanceLogList(logs), total, page, pageSize)
}

// ---- helpers ----

func parsePage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}
