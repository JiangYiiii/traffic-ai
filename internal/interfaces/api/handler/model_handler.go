package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appModel "github.com/trailyai/traffic-ai/internal/application/model"
	appRouting "github.com/trailyai/traffic-ai/internal/application/routing"
	domainModel "github.com/trailyai/traffic-ai/internal/domain/model"
	"github.com/trailyai/traffic-ai/internal/domain/provider"
	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/dto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/httputil"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type ModelHandler struct {
	modelUC      *appModel.UseCase
	routingUC    *appRouting.UseCase
	usageLogRepo *mysql.UsageLogRepo
}

func NewModelHandler(modelUC *appModel.UseCase, routingUC *appRouting.UseCase, usageLogRepo *mysql.UsageLogRepo) *ModelHandler {
	return &ModelHandler{modelUC: modelUC, routingUC: routingUC, usageLogRepo: usageLogRepo}
}

// RegisterUser 注册用户端API路由
func (h *ModelHandler) RegisterUser(group *gin.RouterGroup) {
	group.GET("/models", h.GetListedModels)
	group.GET("/usage-logs", h.ListUserUsageLogs)
	group.GET("/model-pricing", h.ListModelPricing)
	group.GET("/token-groups", h.ListUserTokenGroups)
}

func (h *ModelHandler) Register(group *gin.RouterGroup) {
	group.GET("/providers", h.ListProviders)
	group.POST("/provider-models/discover", h.DiscoverProviderModels)

	group.POST("/models/batch", h.BatchCreateModels)
	group.PUT("/models/batch", h.BatchUpdateModels)
	group.GET("/models", h.ListModels)
	group.POST("/models", h.CreateModel)
	group.PUT("/models/:id", h.UpdateModel)
	group.DELETE("/models/:id", h.DeleteModel)

	group.POST("/models/:id/test", h.TestModel)
	group.POST("/models/:id/playground", h.PlaygroundModel)

	// 新路由：model-accounts
	group.GET("/models/:id/model-accounts", h.ListModelAccounts)
	group.POST("/models/:id/model-accounts", h.CreateModelAccount)
	group.PUT("/model-accounts/:id", h.UpdateModelAccount)
	group.DELETE("/model-accounts/:id", h.DeleteModelAccount)
	group.PATCH("/model-accounts/:id/weight", h.PatchModelAccountWeight)
	group.PATCH("/model-accounts/:id/toggle", h.ToggleModelAccount)
	group.POST("/model-accounts/:id/test", h.TestModelAccount)

	// 旧路由（兼容过渡，前端迁移完成后可删除）
	group.GET("/models/:id/upstreams", h.ListModelAccounts)
	group.POST("/models/:id/upstreams", h.CreateModelAccount)
	group.PUT("/upstreams/:id", h.UpdateModelAccount)
	group.DELETE("/upstreams/:id", h.DeleteModelAccount)
	group.PATCH("/upstreams/:id/weight", h.PatchModelAccountWeight)
	group.PATCH("/upstreams/:id/toggle", h.ToggleModelAccount)

	group.GET("/usage-logs", h.ListUsageLogs)

	group.GET("/token-groups", h.ListTokenGroups)
	group.POST("/token-groups", h.CreateTokenGroup)

	// 新路由：token-group 下的 model-accounts
	group.GET("/token-groups/:id/model-accounts", h.ListGroupModelAccounts)
	group.POST("/token-groups/:id/model-accounts", h.AddGroupModelAccount)
	group.DELETE("/token-groups/:id/model-accounts/:uid", h.RemoveGroupModelAccount)

	// 旧路由（兼容）
	group.GET("/token-groups/:id/upstreams", h.ListGroupModelAccounts)
	group.POST("/token-groups/:id/upstreams", h.AddGroupModelAccount)
	group.DELETE("/token-groups/:id/upstreams/:uid", h.RemoveGroupModelAccount)
}

// ListProviders 返回可选商家及支持的接入方式（先选商家再选 auth 再填密钥）。
func (h *ModelHandler) ListProviders(c *gin.Context) {
	type item struct {
		ID             string   `json:"id"`
		DisplayName    string   `json:"display_name"`
		VendorGroup    string   `json:"vendor_group"`
		AuthTypes      []string `json:"auth_types"`
		DefaultBaseURL string   `json:"default_base_url,omitempty"`
		RequireBaseURL bool     `json:"require_base_url"`
		ListSupported  bool     `json:"list_supported"`
		ProviderTag    string   `json:"provider_tag"`
		OAuthConfigKey string   `json:"oauth_config_key,omitempty"`
	}
	out := make([]item, 0, len(provider.Catalog))
	for _, d := range provider.Catalog {
		listOK := d.Kind != provider.KindManualOnly
		out = append(out, item{
			ID:             d.ID,
			DisplayName:    d.DisplayName,
			VendorGroup:    d.VendorGroup,
			AuthTypes:      d.AuthTypes,
			DefaultBaseURL: d.DefaultBaseURL,
			RequireBaseURL: d.RequireBaseURL,
			ListSupported:  listOK,
			ProviderTag:    d.ProviderTag,
			OAuthConfigKey: d.OAuthConfigKey,
		})
	}
	response.OK(c, gin.H{"providers": out})
}

// DiscoverProviderModels 用当前密钥向远端拉取可用模型 id；失败时仍返回 200 且 fetch_failed=true，便于前端回退手填。
func (h *ModelHandler) DiscoverProviderModels(c *gin.Context) {
	var req dto.DiscoverProviderModelsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	out := h.modelUC.DiscoverRemoteModels(
		c.Request.Context(),
		req.Provider,
		req.AuthType,
		req.Credential,
		req.BaseURL,
	)
	response.OK(c, out)
}

// ---- Model ----

func (h *ModelHandler) ListModels(c *gin.Context) {
	filter := domainModel.ListFilter{
		Provider: strings.TrimSpace(c.Query("provider")),
		NameLike: strings.TrimSpace(c.Query("q")),
	}
	models, err := h.modelUC.ListModels(c.Request.Context(), filter)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToModelItemList(models))
}

func (h *ModelHandler) CreateModel(c *gin.Context) {
	var req dto.CreateModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, http.StatusBadRequest, errcode.ErrBadRequest.Code, httputil.FriendlyJSONBindError(err))
		return
	}
	m := req.ToDomain()
	opts := appModel.BatchOptsFromFields(req.AccountCredential, req.AccountEndpoint)
	if err := h.modelUC.CreateModel(c.Request.Context(), m, opts); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToModelItem(m))
}

func (h *ModelHandler) BatchCreateModels(c *gin.Context) {
	var req dto.BatchCreateModelsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, http.StatusBadRequest, errcode.ErrBadRequest.Code, httputil.FriendlyJSONBindError(err))
		return
	}
	items := make([]appModel.BatchModelItem, 0, len(req.Items))
	for i := range req.Items {
		items = append(items, appModel.BatchModelItem{
			Model: req.Items[i].ToDomain(),
			Opts:  appModel.BatchOptsFromFields(req.Items[i].AccountCredential, req.Items[i].AccountEndpoint),
		})
	}
	out := h.modelUC.BatchCreateModels(c.Request.Context(), items)
	created := make([]dto.ModelItem, 0, len(out.Created))
	for _, m := range out.Created {
		created = append(created, dto.ToModelItem(m))
	}
	failed := make([]dto.BatchCreateFailure, 0, len(out.Failed))
	for _, f := range out.Failed {
		failed = append(failed, dto.BatchCreateFailure{Index: f.Index, ModelName: f.ModelName, Message: f.Message})
	}
	response.OK(c, dto.BatchCreateModelsResult{Created: created, Failed: failed})
}

func (h *ModelHandler) BatchUpdateModels(c *gin.Context) {
	var req dto.BatchUpdateModelsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, http.StatusBadRequest, errcode.ErrBadRequest.Code, httputil.FriendlyJSONBindError(err))
		return
	}
	if err := h.modelUC.BatchUpdateModels(
		c.Request.Context(),
		req.IDs,
		req.BillingType,
		req.InputPrice,
		req.OutputPrice,
		req.ReasoningPrice,
		req.PerRequestPrice,
	); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "updated")
}

func (h *ModelHandler) UpdateModel(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var req dto.UpdateModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, http.StatusBadRequest, errcode.ErrBadRequest.Code, httputil.FriendlyJSONBindError(err))
		return
	}
	m := req.ToDomain(id)
	if err := h.modelUC.UpdateModel(c.Request.Context(), m); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "updated")
}

func (h *ModelHandler) DeleteModel(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	if err := h.modelUC.DeleteModel(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "deleted")
}

func (h *ModelHandler) TestModel(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	result, err := h.modelUC.TestModelConnectivity(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *ModelHandler) TestModelAccount(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	result, err := h.modelUC.TestModelAccountConnectivity(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *ModelHandler) PlaygroundModel(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var req dto.PlaygroundReq
	_ = c.ShouldBindJSON(&req)
	result, err := h.modelUC.PlaygroundChat(c.Request.Context(), id, req.Messages, req.MaxTokens)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, result)
}

// ---- ModelAccount ----

func (h *ModelHandler) ListModelAccounts(c *gin.Context) {
	modelID, ok := parsePathID(c)
	if !ok {
		return
	}
	list, err := h.modelUC.ListModelAccounts(c.Request.Context(), modelID)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToModelAccountItemList(list))
}

func (h *ModelHandler) CreateModelAccount(c *gin.Context) {
	modelID, ok := parsePathID(c)
	if !ok {
		return
	}
	var req dto.CreateModelAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, http.StatusBadRequest, errcode.ErrBadRequest.Code, httputil.FriendlyJSONBindError(err))
		return
	}
	a := req.ToDomain(modelID)
	if err := h.modelUC.CreateModelAccount(c.Request.Context(), a); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{"id": a.ID})
}

func (h *ModelHandler) UpdateModelAccount(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var req dto.UpdateModelAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailMsg(c, http.StatusBadRequest, errcode.ErrBadRequest.Code, httputil.FriendlyJSONBindError(err))
		return
	}
	a := req.ToDomain(id)
	if err := h.modelUC.UpdateModelAccount(c.Request.Context(), a); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "updated")
}

func (h *ModelHandler) DeleteModelAccount(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	if err := h.modelUC.DeleteModelAccount(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "deleted")
}

// ---- TokenGroup ----

func (h *ModelHandler) ListTokenGroups(c *gin.Context) {
	groups, err := h.routingUC.ListTokenGroups(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToTokenGroupItemList(groups))
}

// ListUserTokenGroups 面向普通用户：返回当前可选的令牌分组精简列表，
// 仅包含 is_active=true 的分组，且只暴露 name/description。
func (h *ModelHandler) ListUserTokenGroups(c *gin.Context) {
	groups, err := h.routingUC.ListTokenGroups(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{"items": dto.ToUserTokenGroupItemList(groups)})
}

func (h *ModelHandler) CreateTokenGroup(c *gin.Context) {
	var req dto.CreateTokenGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	tg := req.ToDomain()
	if err := h.routingUC.CreateTokenGroup(c.Request.Context(), tg); err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToTokenGroupItem(tg))
}

func (h *ModelHandler) ListGroupModelAccounts(c *gin.Context) {
	groupID, ok := parsePathID(c)
	if !ok {
		return
	}
	ids, err := h.routingUC.ListModelAccountIDsForGroup(c.Request.Context(), groupID)
	if err != nil {
		handleError(c, err)
		return
	}
	// 保留旧字段名 upstream_ids 便于前端灰度迁移，同时提供新字段。
	response.OK(c, gin.H{"model_account_ids": ids, "upstream_ids": ids})
}

func (h *ModelHandler) AddGroupModelAccount(c *gin.Context) {
	groupID, ok := parsePathID(c)
	if !ok {
		return
	}
	// 兼容旧字段 upstream_id：前端未迁移前仍可用。
	var req struct {
		ModelAccountID int64 `json:"model_account_id"`
		UpstreamID     int64 `json:"upstream_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	id := req.ModelAccountID
	if id == 0 {
		id = req.UpstreamID
	}
	if id == 0 {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	if err := h.routingUC.AddModelAccountToGroup(c.Request.Context(), groupID, id); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "added")
}

func (h *ModelHandler) RemoveGroupModelAccount(c *gin.Context) {
	groupID, ok := parsePathID(c)
	if !ok {
		return
	}
	uidStr := c.Param("uid")
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	if err := h.routingUC.RemoveModelAccountFromGroup(c.Request.Context(), groupID, uid); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "removed")
}

// ---- Usage Logs ----

func (h *ModelHandler) ListUsageLogs(c *gin.Context) {
	page, pageSize := parsePage(c)
	filter := &mysql.UsageLogFilter{
		Model:  c.Query("model"),
		Status: c.Query("status"),
	}
	logs, total, err := h.usageLogRepo.ListPaged(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		handleError(c, errcode.ErrInternal)
		return
	}
	response.PageResult(c, dto.ToUsageLogList(logs), total, page, pageSize)
}

// ---- helpers ----

func parsePathID(c *gin.Context) (int64, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return 0, false
	}
	return id, true
}

// ListUserUsageLogs 用户端调用日志（仅当前用户数据）。
//
// Query 参数：
//   - page / page_size：分页（由 parsePage 处理）
//   - model / status：精确筛选
//   - stream：取值 "true" / "false" 启用流式开关筛选；其他值忽略
//   - start_time / end_time：RFC3339 / RFC3339Nano 时间字符串，对 created_at 做闭区间过滤；
//     空字符串表示不过滤，格式非法则返回 400，end_time 早于 start_time 也返回 400
func (h *ModelHandler) ListUserUsageLogs(c *gin.Context) {
	uid, ok := getUserID(c)
	if !ok {
		return
	}
	page, pageSize := parsePage(c)
	filter := &mysql.UsageLogFilter{
		UserID: uid,
		Model:  strings.TrimSpace(c.Query("model")),
		Status: strings.TrimSpace(c.Query("status")),
	}
	streamQ := strings.TrimSpace(c.Query("stream"))
	if streamQ == "true" || streamQ == "false" {
		filter.IsStreamFilter = &streamQ
	}
	startTime, ok := parseOptionalTimeQuery(c, "start_time")
	if !ok {
		return
	}
	endTime, ok := parseOptionalTimeQuery(c, "end_time")
	if !ok {
		return
	}
	if startTime != nil && endTime != nil && endTime.Before(*startTime) {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}
	filter.StartTime = startTime
	filter.EndTime = endTime
	logs, total, err := h.usageLogRepo.ListPagedWithToken(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		handleError(c, errcode.ErrInternal)
		return
	}
	response.PageResult(c, dto.ToUserUsageLogList(logs), total, page, pageSize)
}

// ListModelPricing 用户端模型定价列表（已上架且有定价的模型）。
func (h *ModelHandler) ListModelPricing(c *gin.Context) {
	models, err := h.modelUC.GetListedModels(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, dto.ToModelPricingList(models))
}

// GetListedModels 获取已上架模型列表 (用户端API)
func (h *ModelHandler) GetListedModels(c *gin.Context) {
	models, err := h.modelUC.GetListedModels(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}

	resp := make([]dto.ModelResp, 0, len(models))
	for _, m := range models {
		resp = append(resp, dto.ModelResp{
			ID:              m.ID,
			ModelName:       m.ModelName,
			Provider:        m.Provider,
			ModelType:       m.ModelType,
			BillingType:     string(m.BillingType),
			InputPrice:      m.InputPrice,
			OutputPrice:     m.OutputPrice,
			ReasoningPrice:  m.ReasoningPrice,
			PerRequestPrice: m.PerRequestPrice,
			IsActive:        m.IsActive,
			IsListed:        m.IsListed,
		})
	}

	response.OK(c, resp)
}

// ---- ModelAccount 快捷操作 ----

// PatchModelAccountWeight 快速调整账号权重（0-100）。
// PATCH /admin/model-accounts/:id/weight  body: {"weight": 50}
func (h *ModelHandler) PatchModelAccountWeight(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var req struct {
		Weight int `json:"weight" binding:"min=0,max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	existing, err := h.modelUC.FindModelAccount(c.Request.Context(), id)
	if err != nil || existing == nil {
		handleError(c, errcode.ErrModelAccountNotFound)
		return
	}
	existing.Weight = req.Weight
	existing.Credential = "" // 不修改凭证
	if err := h.modelUC.UpdateModelAccount(c.Request.Context(), existing); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "weight updated")
}

// ToggleModelAccount 快速启停账号。
// PATCH /admin/model-accounts/:id/toggle  body: {"is_active": true}
func (h *ModelHandler) ToggleModelAccount(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	existing, err := h.modelUC.FindModelAccount(c.Request.Context(), id)
	if err != nil || existing == nil {
		handleError(c, errcode.ErrModelAccountNotFound)
		return
	}
	existing.IsActive = req.IsActive
	existing.Credential = "" // 不修改凭证
	if err := h.modelUC.UpdateModelAccount(c.Request.Context(), existing); err != nil {
		handleError(c, err)
		return
	}
	response.OKMsg(c, "toggled")
}
