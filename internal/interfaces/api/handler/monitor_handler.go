package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appmonitor "github.com/trailyai/traffic-ai/internal/application/monitor"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

// MonitorHandler 流量监控 API handler。
type MonitorHandler struct {
	uc *appmonitor.UseCase
}

func NewMonitorHandler(uc *appmonitor.UseCase) *MonitorHandler {
	return &MonitorHandler{uc: uc}
}

// Register 注册监控路由（挂在 super_admin 组下）。
func (h *MonitorHandler) Register(group *gin.RouterGroup) {
	group.GET("/monitor/overview", h.Overview)
	group.GET("/monitor/models/:id", h.ModelDetail)
	group.GET("/monitor/accounts/:id", h.AccountDetail)
	group.GET("/monitor/realtime", h.Realtime)
}

// Overview 返回所有模型聚合指标总览。
// GET /admin/monitor/overview?hours=1
func (h *MonitorHandler) Overview(c *gin.Context) {
	hours := parseIntQuery(c, "hours", 1)
	result, err := h.uc.GetOverview(c.Request.Context(), hours)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, result)
}

// ModelDetail 返回单模型详情 + 按账号拆分 + 时间趋势。
// GET /admin/monitor/models/:id?hours=1&granularity=hour
func (h *MonitorHandler) ModelDetail(c *gin.Context) {
	modelID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || modelID <= 0 {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	hours := parseIntQuery(c, "hours", 1)
	granularity := c.DefaultQuery("granularity", "hour")

	result, err := h.uc.GetModelDetail(c.Request.Context(), modelID, hours, granularity)
	if err != nil {
		handleError(c, err)
		return
	}
	if result == nil {
		response.FailMsg(c, http.StatusNotFound, 40400, "model not found")
		return
	}
	response.OK(c, result)
}

// AccountDetail 返回单账号详情 + 时间趋势。
// GET /admin/monitor/accounts/:id?hours=24&granularity=hour
func (h *MonitorHandler) AccountDetail(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	hours := parseIntQuery(c, "hours", 24)
	granularity := c.DefaultQuery("granularity", "hour")

	result, err := h.uc.GetAccountDetail(c.Request.Context(), accountID, hours, granularity)
	if err != nil {
		handleError(c, err)
		return
	}
	if result == nil {
		response.FailMsg(c, http.StatusNotFound, 40400, "account not found")
		return
	}
	response.OK(c, result)
}

// Realtime 返回 Redis 今日实时快照（全模型 + 全账号）。
// GET /admin/monitor/realtime
func (h *MonitorHandler) Realtime(c *gin.Context) {
	result, err := h.uc.GetRealtime(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, result)
}

func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	v := c.Query(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
