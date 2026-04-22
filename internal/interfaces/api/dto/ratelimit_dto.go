package dto

import (
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/ratelimit"
)

// ---------- Request ----------

type CreateRateLimitRuleReq struct {
	Name          string `json:"name" binding:"required,max=100"`
	Scope         string `json:"scope" binding:"required,oneof=global user api_key model"`
	ScopeValue    string `json:"scope_value"`
	MaxRPM        int    `json:"max_rpm" binding:"min=0"`
	MaxTPM        int    `json:"max_tpm" binding:"min=0"`
	MaxConcurrent int    `json:"max_concurrent" binding:"min=0"`
	IsActive      *bool  `json:"is_active"`
}

type UpdateRateLimitRuleReq struct {
	Name          string `json:"name" binding:"required,max=100"`
	Scope         string `json:"scope" binding:"required,oneof=global user api_key model"`
	ScopeValue    string `json:"scope_value"`
	MaxRPM        int    `json:"max_rpm" binding:"min=0"`
	MaxTPM        int    `json:"max_tpm" binding:"min=0"`
	MaxConcurrent int    `json:"max_concurrent" binding:"min=0"`
	IsActive      *bool  `json:"is_active"`
}

func (r *CreateRateLimitRuleReq) ToDomain() *domain.RateLimitRule {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	return &domain.RateLimitRule{
		Name:          r.Name,
		Scope:         domain.Scope(r.Scope),
		ScopeValue:    r.ScopeValue,
		MaxRPM:        r.MaxRPM,
		MaxTPM:        r.MaxTPM,
		MaxConcurrent: r.MaxConcurrent,
		IsActive:      active,
	}
}

func (r *UpdateRateLimitRuleReq) ToDomain(id int64) *domain.RateLimitRule {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	return &domain.RateLimitRule{
		ID:            id,
		Name:          r.Name,
		Scope:         domain.Scope(r.Scope),
		ScopeValue:    r.ScopeValue,
		MaxRPM:        r.MaxRPM,
		MaxTPM:        r.MaxTPM,
		MaxConcurrent: r.MaxConcurrent,
		IsActive:      active,
	}
}

// ---------- Response ----------

type RateLimitRuleItem struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Scope         string `json:"scope"`
	ScopeValue    string `json:"scope_value"`
	MaxRPM        int    `json:"max_rpm"`
	MaxTPM        int    `json:"max_tpm"`
	MaxConcurrent int    `json:"max_concurrent"`
	IsActive      bool   `json:"is_active"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func ToRateLimitRuleItem(rule *domain.RateLimitRule) RateLimitRuleItem {
	return RateLimitRuleItem{
		ID:            rule.ID,
		Name:          rule.Name,
		Scope:         string(rule.Scope),
		ScopeValue:    rule.ScopeValue,
		MaxRPM:        rule.MaxRPM,
		MaxTPM:        rule.MaxTPM,
		MaxConcurrent: rule.MaxConcurrent,
		IsActive:      rule.IsActive,
		CreatedAt:     rule.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     rule.UpdatedAt.Format(time.RFC3339),
	}
}

func ToRateLimitRuleList(rules []*domain.RateLimitRule) []RateLimitRuleItem {
	items := make([]RateLimitRuleItem, 0, len(rules))
	for _, r := range rules {
		items = append(items, ToRateLimitRuleItem(r))
	}
	return items
}
