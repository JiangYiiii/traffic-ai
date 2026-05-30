package dto

import domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"

type AutoRoutePolicyReq struct {
	VirtualModelID int64  `json:"virtual_model_id" binding:"required"`
	Name           string `json:"name" binding:"required"`
	Strategy       string `json:"strategy" binding:"required"`
	RulesJSON      string `json:"rules_json"`
	IsActive       *bool  `json:"is_active"`
	Version        int    `json:"version"`
}

func (r AutoRoutePolicyReq) ToDomain(id int64) *domainRouting.AutoRoutePolicy {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	return &domainRouting.AutoRoutePolicy{
		ID:             id,
		VirtualModelID: r.VirtualModelID,
		Name:           r.Name,
		Strategy:       r.Strategy,
		RulesJSON:      r.RulesJSON,
		IsActive:       active,
		Version:        r.Version,
	}
}

type AutoRoutePolicyItem struct {
	ID             int64  `json:"id"`
	VirtualModelID int64  `json:"virtual_model_id"`
	Name           string `json:"name"`
	Strategy       string `json:"strategy"`
	RulesJSON      string `json:"rules_json"`
	IsActive       bool   `json:"is_active"`
	Version        int    `json:"version"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func ToAutoRoutePolicyItem(p *domainRouting.AutoRoutePolicy) AutoRoutePolicyItem {
	return AutoRoutePolicyItem{
		ID:             p.ID,
		VirtualModelID: p.VirtualModelID,
		Name:           p.Name,
		Strategy:       p.Strategy,
		RulesJSON:      p.RulesJSON,
		IsActive:       p.IsActive,
		Version:        p.Version,
		CreatedAt:      p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func ToAutoRoutePolicyItems(list []*domainRouting.AutoRoutePolicy) []AutoRoutePolicyItem {
	out := make([]AutoRoutePolicyItem, 0, len(list))
	for _, p := range list {
		out = append(out, ToAutoRoutePolicyItem(p))
	}
	return out
}

type AutoRouteCandidateReq struct {
	TargetModelID           int64 `json:"target_model_id" binding:"required"`
	Priority                int   `json:"priority"`
	Weight                  int   `json:"weight"`
	MinRequestContextTokens int   `json:"min_request_context_tokens"`
	QualityScore            int   `json:"quality_score"`
	CostBias                int   `json:"cost_bias"`
	LatencyBias             int   `json:"latency_bias"`
	IsActive                *bool `json:"is_active"`
}

func (r AutoRouteCandidateReq) ToDomain(id, policyID int64) *domainRouting.AutoRouteCandidate {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	weight := r.Weight
	if weight == 0 {
		weight = 1
	}
	quality := r.QualityScore
	if quality == 0 {
		quality = 50
	}
	return &domainRouting.AutoRouteCandidate{
		ID:                      id,
		PolicyID:                policyID,
		TargetModelID:           r.TargetModelID,
		Priority:                r.Priority,
		Weight:                  weight,
		MinRequestContextTokens: r.MinRequestContextTokens,
		QualityScore:            quality,
		CostBias:                r.CostBias,
		LatencyBias:             r.LatencyBias,
		IsActive:                active,
	}
}

type AutoRouteCandidateItem struct {
	ID                      int64  `json:"id"`
	PolicyID                int64  `json:"policy_id"`
	TargetModelID           int64  `json:"target_model_id"`
	Priority                int    `json:"priority"`
	Weight                  int    `json:"weight"`
	MinRequestContextTokens int    `json:"min_request_context_tokens"`
	QualityScore            int    `json:"quality_score"`
	CostBias                int    `json:"cost_bias"`
	LatencyBias             int    `json:"latency_bias"`
	IsActive                bool   `json:"is_active"`
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
}

func ToAutoRouteCandidateItem(c *domainRouting.AutoRouteCandidate) AutoRouteCandidateItem {
	return AutoRouteCandidateItem{
		ID:                      c.ID,
		PolicyID:                c.PolicyID,
		TargetModelID:           c.TargetModelID,
		Priority:                c.Priority,
		Weight:                  c.Weight,
		MinRequestContextTokens: c.MinRequestContextTokens,
		QualityScore:            c.QualityScore,
		CostBias:                c.CostBias,
		LatencyBias:             c.LatencyBias,
		IsActive:                c.IsActive,
		CreatedAt:               c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:               c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func ToAutoRouteCandidateItems(list []*domainRouting.AutoRouteCandidate) []AutoRouteCandidateItem {
	out := make([]AutoRouteCandidateItem, 0, len(list))
	for _, c := range list {
		out = append(out, ToAutoRouteCandidateItem(c))
	}
	return out
}

type AutoRouteDryRunReq struct {
	TokenGroup      string `json:"token_group"`
	RequestedModel  string `json:"requested_model"`
	Protocol        string `json:"protocol"`
	EstimatedTokens int    `json:"estimated_tokens"`
	Stream          bool   `json:"stream"`
}
