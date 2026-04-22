package dto

import (
	"fmt"
	"time"

	domainModel "github.com/trailyai/traffic-ai/internal/domain/model"
	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
)

// ---- Model ----

type CreateModelReq struct {
	ModelName       string `json:"model_name" binding:"required,max=100"`
	Provider        string `json:"provider" binding:"required,max=50"`
	ModelType       string `json:"model_type" binding:"max=30"`
	BillingType     string `json:"billing_type" binding:"max=20"`
	InputPrice      int64  `json:"input_price"`
	OutputPrice     int64  `json:"output_price"`
	ReasoningPrice  int64  `json:"reasoning_price"`
	PerRequestPrice int64  `json:"per_request_price"`
	IsActive        *bool  `json:"is_active"`
	IsListed        *bool  `json:"is_listed"`
	// AccountCredential 非空时，创建模型后自动创建一条默认模型账号（内置商家使用目录默认端点；自定义商家须配合 AccountEndpoint）。
	AccountCredential string `json:"account_credential"`
	AccountEndpoint   string `json:"account_endpoint"`
}

// BatchCreateModelsReq 批量创建模型（逐项独立校验与落库，失败项记入 failed，不中断其余项）。
type BatchCreateModelsReq struct {
	Items []CreateModelReq `json:"items" binding:"required,min=1,max=200"`
}

// BatchCreateFailure 单条失败说明。
type BatchCreateFailure struct {
	Index     int    `json:"index"`
	ModelName string `json:"model_name,omitempty"`
	Message   string `json:"message"`
}

// BatchCreateModelsResult 批量创建结果。
type BatchCreateModelsResult struct {
	Created []ModelItem          `json:"created"`
	Failed  []BatchCreateFailure `json:"failed"`
}

// BatchUpdateModelsReq 批量修改计费字段（应用到 ids 中每一条）。
type BatchUpdateModelsReq struct {
	IDs             []int64 `json:"ids" binding:"required,min=1"`
	BillingType     string  `json:"billing_type" binding:"required"`
	InputPrice      int64   `json:"input_price"`
	OutputPrice     int64   `json:"output_price"`
	ReasoningPrice  int64   `json:"reasoning_price"`
	PerRequestPrice int64   `json:"per_request_price"`
}

// PlaygroundReq 管理端 Playground：向该模型绑定的模型账号发一条 chat/completions。
type PlaygroundReq struct {
	Messages  []map[string]string `json:"messages"`
	MaxTokens int                 `json:"max_tokens"`
}

func (r *CreateModelReq) ToDomain() *domainModel.Model {
	m := &domainModel.Model{
		ModelName:       r.ModelName,
		Provider:        r.Provider,
		ModelType:       r.ModelType,
		BillingType:     domainModel.BillingType(r.BillingType),
		InputPrice:      r.InputPrice,
		OutputPrice:     r.OutputPrice,
		ReasoningPrice:  r.ReasoningPrice,
		PerRequestPrice: r.PerRequestPrice,
		IsActive:        true,
		IsListed:        false, // 默认不上架
	}
	if m.ModelType == "" {
		m.ModelType = "chat"
	}
	if m.BillingType == "" {
		m.BillingType = domainModel.BillingPerToken
	}
	if r.IsActive != nil {
		m.IsActive = *r.IsActive
	}
	if r.IsListed != nil {
		m.IsListed = *r.IsListed
	}
	return m
}

type UpdateModelReq struct {
	ModelName       string `json:"model_name" binding:"required,max=100"`
	Provider        string `json:"provider" binding:"required,max=50"`
	ModelType       string `json:"model_type"`
	BillingType     string `json:"billing_type"`
	InputPrice      int64  `json:"input_price"`
	OutputPrice     int64  `json:"output_price"`
	ReasoningPrice  int64  `json:"reasoning_price"`
	PerRequestPrice int64  `json:"per_request_price"`
	IsActive        *bool  `json:"is_active"`
	IsListed        *bool  `json:"is_listed"`
}

func (r *UpdateModelReq) ToDomain(id int64) *domainModel.Model {
	m := &domainModel.Model{
		ID:              id,
		ModelName:       r.ModelName,
		Provider:        r.Provider,
		ModelType:       r.ModelType,
		BillingType:     domainModel.BillingType(r.BillingType),
		InputPrice:      r.InputPrice,
		OutputPrice:     r.OutputPrice,
		ReasoningPrice:  r.ReasoningPrice,
		PerRequestPrice: r.PerRequestPrice,
		IsActive:        true,
		IsListed:        false,
	}
	if r.IsActive != nil {
		m.IsActive = *r.IsActive
	}
	if r.IsListed != nil {
		m.IsListed = *r.IsListed
	}
	return m
}

type ModelItem struct {
	ID                int64  `json:"id"`
	ModelName         string `json:"model_name"`
	Provider          string `json:"provider"`
	ModelType         string `json:"model_type"`
	BillingType       string `json:"billing_type"`
	InputPrice        int64  `json:"input_price"`
	OutputPrice       int64  `json:"output_price"`
	ReasoningPrice    int64  `json:"reasoning_price"`
	PerRequestPrice   int64  `json:"per_request_price"`
	IsActive          bool   `json:"is_active"`
	IsListed          bool   `json:"is_listed"`
	LastTestPassed    *bool  `json:"last_test_passed"`
	LastTestAt        string `json:"last_test_at,omitempty"`
	LastTestLatencyMs *int   `json:"last_test_latency_ms,omitempty"`
	LastTestError     string `json:"last_test_error,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

func ToModelItem(m *domainModel.Model) ModelItem {
	item := ModelItem{
		ID:                m.ID,
		ModelName:         m.ModelName,
		Provider:          m.Provider,
		ModelType:         m.ModelType,
		BillingType:       string(m.BillingType),
		InputPrice:        m.InputPrice,
		OutputPrice:       m.OutputPrice,
		ReasoningPrice:    m.ReasoningPrice,
		PerRequestPrice:   m.PerRequestPrice,
		IsActive:          m.IsActive,
		IsListed:          m.IsListed,
		LastTestPassed:    m.LastTestPassed,
		LastTestLatencyMs: m.LastTestLatencyMs,
		LastTestError:     m.LastTestError,
		CreatedAt:         m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         m.UpdatedAt.Format(time.RFC3339),
	}
	if m.LastTestAt != nil {
		item.LastTestAt = m.LastTestAt.UTC().Format(time.RFC3339)
	}
	return item
}

func ToModelItemList(models []*domainModel.Model) []ModelItem {
	items := make([]ModelItem, 0, len(models))
	for _, m := range models {
		items = append(items, ToModelItem(m))
	}
	return items
}

// ModelResp 用户端模型响应 (简化版，只包含用户需要的字段)
type ModelResp struct {
	ID              int64  `json:"id"`
	ModelName       string `json:"model_name"`
	Provider        string `json:"provider"`
	ModelType       string `json:"model_type"`
	BillingType     string `json:"billing_type"`
	InputPrice      int64  `json:"input_price"`
	OutputPrice     int64  `json:"output_price"`
	ReasoningPrice  int64  `json:"reasoning_price"`
	PerRequestPrice int64  `json:"per_request_price"`
	IsActive        bool   `json:"is_active"`
	IsListed        bool   `json:"is_listed"`
}

// ---- ModelAccount ----

type CreateModelAccountReq struct {
	Name       string `json:"name" binding:"max=100"`
	Provider   string `json:"provider" binding:"max=50"`
	AuthType   string `json:"auth_type" binding:"omitempty,oneof=api_key oauth_authorization_code"`
	Endpoint   string `json:"endpoint" binding:"required,url,max=500"`
	Credential string `json:"credential" binding:"required"`
	Protocol   string `json:"protocol" binding:"max=20"`
	Weight     int    `json:"weight"`
	IsActive   *bool  `json:"is_active"`
	TimeoutSec int    `json:"timeout_sec"`
}

func (r *CreateModelAccountReq) ToDomain(modelID int64) *domainModel.ModelAccount {
	a := &domainModel.ModelAccount{
		ModelID:    modelID,
		Name:       r.Name,
		Provider:   r.Provider,
		Endpoint:   r.Endpoint,
		Credential: r.Credential,
		Protocol:   r.Protocol,
		Weight:     r.Weight,
		IsActive:   true,
		TimeoutSec: r.TimeoutSec,
		AuthType:   "api_key",
	}
	if r.AuthType != "" {
		a.AuthType = r.AuthType
	}
	if a.Protocol == "" {
		a.Protocol = "chat"
	}
	if a.Weight <= 0 {
		a.Weight = 1
	}
	if a.TimeoutSec <= 0 {
		a.TimeoutSec = 60
	}
	if r.IsActive != nil {
		a.IsActive = *r.IsActive
	}
	return a
}

type UpdateModelAccountReq struct {
	Name       string `json:"name" binding:"max=100"`
	Provider   string `json:"provider" binding:"max=50"`
	AuthType   string `json:"auth_type" binding:"omitempty,oneof=api_key oauth_authorization_code"`
	Endpoint   string `json:"endpoint" binding:"required,url,max=500"`
	Credential string `json:"credential"`
	Protocol   string `json:"protocol" binding:"max=20"`
	Weight     int    `json:"weight"`
	IsActive   *bool  `json:"is_active"`
	TimeoutSec int    `json:"timeout_sec"`
}

func (r *UpdateModelAccountReq) ToDomain(id int64) *domainModel.ModelAccount {
	a := &domainModel.ModelAccount{
		ID:         id,
		Name:       r.Name,
		Provider:   r.Provider,
		AuthType:   r.AuthType,
		Endpoint:   r.Endpoint,
		Credential: r.Credential,
		Protocol:   r.Protocol,
		Weight:     r.Weight,
		IsActive:   true,
		TimeoutSec: r.TimeoutSec,
	}
	if a.Weight <= 0 {
		a.Weight = 1
	}
	if a.TimeoutSec <= 0 {
		a.TimeoutSec = 60
	}
	if r.IsActive != nil {
		a.IsActive = *r.IsActive
	}
	return a
}

type ModelAccountItem struct {
	ID                int64  `json:"id"`
	ModelID           int64  `json:"model_id"`
	Name              string `json:"name"`
	Provider          string `json:"provider"`
	Endpoint          string `json:"endpoint"`
	Credential        string `json:"credential"`
	Protocol          string `json:"protocol"`
	Weight            int    `json:"weight"`
	IsActive          bool   `json:"is_active"`
	Status            string `json:"status"`
	AuthType          string `json:"auth_type"`
	TimeoutSec        int    `json:"timeout_sec"`
	LastTestPassed    *bool  `json:"last_test_passed"`
	LastTestAt        string `json:"last_test_at,omitempty"`
	LastTestLatencyMs *int   `json:"last_test_latency_ms,omitempty"`
	LastTestError     string `json:"last_test_error,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

func ToModelAccountItem(a *domainModel.ModelAccount) ModelAccountItem {
	item := ModelAccountItem{
		ID:             a.ID,
		ModelID:        a.ModelID,
		Name:           a.Name,
		Provider:       a.Provider,
		Endpoint:       a.Endpoint,
		Credential:     maskCredential(a.Credential),
		Protocol:       a.Protocol,
		Weight:         a.Weight,
		IsActive:       a.IsActive,
		Status:         a.Status(),
		AuthType:       a.AuthType,
		TimeoutSec:     a.TimeoutSec,
		LastTestPassed: a.LastTestPassed,
		LastTestError:  a.LastTestError,
		CreatedAt:      a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
	}
	if a.LastTestAt != nil {
		item.LastTestAt = a.LastTestAt.UTC().Format(time.RFC3339)
	}
	if a.LastTestLatencyMs != nil {
		item.LastTestLatencyMs = a.LastTestLatencyMs
	}
	return item
}

func ToModelAccountItemList(accounts []*domainModel.ModelAccount) []ModelAccountItem {
	items := make([]ModelAccountItem, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, ToModelAccountItem(a))
	}
	return items
}

func maskCredential(cred string) string {
	if len(cred) <= 8 {
		return "****"
	}
	return cred[:4] + "****" + cred[len(cred)-4:]
}

// ---- TokenGroup ----

type CreateTokenGroupReq struct {
	Name        string `json:"name" binding:"required,max=50"`
	Description string `json:"description" binding:"max=255"`
	IsActive    *bool  `json:"is_active"`
}

func (r *CreateTokenGroupReq) ToDomain() *domainRouting.TokenGroup {
	tg := &domainRouting.TokenGroup{
		Name:        r.Name,
		Description: r.Description,
		IsActive:    true,
	}
	if r.IsActive != nil {
		tg.IsActive = *r.IsActive
	}
	return tg
}

type TokenGroupItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func ToTokenGroupItem(tg *domainRouting.TokenGroup) TokenGroupItem {
	return TokenGroupItem{
		ID:          tg.ID,
		Name:        tg.Name,
		Description: tg.Description,
		IsActive:    tg.IsActive,
		CreatedAt:   tg.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   tg.UpdatedAt.Format(time.RFC3339),
	}
}

func ToTokenGroupItemList(groups []*domainRouting.TokenGroup) []TokenGroupItem {
	items := make([]TokenGroupItem, 0, len(groups))
	for _, g := range groups {
		items = append(items, ToTokenGroupItem(g))
	}
	return items
}

// UserTokenGroupItem 面向普通用户的精简视图：只暴露可选分组的显示字段，
// 不返回 id、时间戳等内部字段，避免管理侧信息泄露。
type UserTokenGroupItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ToUserTokenGroupItemList 过滤出 is_active=true 的分组并转成精简视图。
func ToUserTokenGroupItemList(groups []*domainRouting.TokenGroup) []UserTokenGroupItem {
	items := make([]UserTokenGroupItem, 0, len(groups))
	for _, g := range groups {
		if g == nil || !g.IsActive {
			continue
		}
		items = append(items, UserTokenGroupItem{
			Name:        g.Name,
			Description: g.Description,
		})
	}
	return items
}

type AddGroupModelAccountReq struct {
	ModelAccountID int64 `json:"model_account_id" binding:"required"`
}

// ---- Model Pricing (user-facing) ----

type ModelPricingItem struct {
	Model          string `json:"model"`
	PricingMode    string `json:"pricingMode"`
	InputUsdPer1M  string `json:"inputUsdPer1M,omitempty"`
	OutputUsdPer1M string `json:"outputUsdPer1M,omitempty"`
	PerRequestUsd  string `json:"perRequestUsd,omitempty"`
}

func microUsdToUsdPer1M(microPerToken int64) string {
	usd := float64(microPerToken) / 1_000_000.0
	return formatFloat(usd)
}

func microUsdToUsd(micro int64) string {
	usd := float64(micro) / 1_000_000.0
	return formatFloat(usd)
}

func formatFloat(f float64) string {
	s := fmt.Sprintf("%.6f", f)
	for len(s) > 1 && s[len(s)-1] == '0' && s[len(s)-2] != '.' {
		s = s[:len(s)-1]
	}
	return s
}

func ToModelPricingItem(m *domainModel.Model) ModelPricingItem {
	item := ModelPricingItem{
		Model: m.ModelName,
	}
	if m.BillingType == domainModel.BillingPerRequest {
		item.PricingMode = "per_request"
		item.PerRequestUsd = microUsdToUsd(m.PerRequestPrice)
	} else {
		item.PricingMode = "per_token"
		item.InputUsdPer1M = microUsdToUsdPer1M(m.InputPrice)
		item.OutputUsdPer1M = microUsdToUsdPer1M(m.OutputPrice)
	}
	return item
}

func ToModelPricingList(models []*domainModel.Model) []ModelPricingItem {
	items := make([]ModelPricingItem, 0, len(models))
	for _, m := range models {
		items = append(items, ToModelPricingItem(m))
	}
	return items
}
