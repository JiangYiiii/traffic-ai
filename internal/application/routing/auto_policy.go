package routing

import (
	"context"
	"encoding/json"
	"strings"

	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

var allowedAutoStrategies = map[string]struct{}{
	domainRouting.AutoStrategyBalanced:  {},
	domainRouting.AutoStrategyFast:      {},
	domainRouting.AutoStrategyCheap:     {},
	domainRouting.AutoStrategyQuality:   {},
	domainRouting.AutoStrategyCoding:    {},
	domainRouting.AutoStrategyReasoning: {},
}

var allowedCapabilityTags = map[string]struct{}{
	"streaming":    {},
	"coding":       {},
	"reasoning":    {},
	"long_context": {},
	"tool_calling": {},
	"json_schema":  {},
	"vision":       {},
	"chat":         {},
	"responses":    {},
	"messages":     {},
	"gemini":       {},
	"embeddings":   {},
	"image":        {},
	"speech":       {},
}

func (uc *UseCase) CreateAutoRoutePolicy(ctx context.Context, p *domainRouting.AutoRoutePolicy) error {
	if uc.autoRepo == nil || !validAutoPolicy(p) {
		return errcode.ErrBadRequest
	}
	if err := validateAutoRulesJSON(p.RulesJSON); err != nil {
		return err
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	if err := uc.autoRepo.CreatePolicy(ctx, p); err != nil {
		logger.L.Errorw("create auto route policy failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) ListAutoRoutePolicies(ctx context.Context) ([]*domainRouting.AutoRoutePolicy, error) {
	if uc.autoRepo == nil {
		return nil, errcode.ErrInternal
	}
	policies, err := uc.autoRepo.ListPolicies(ctx)
	if err != nil {
		logger.L.Errorw("list auto route policies failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return policies, nil
}

func (uc *UseCase) UpdateAutoRoutePolicy(ctx context.Context, p *domainRouting.AutoRoutePolicy) error {
	if uc.autoRepo == nil || p.ID <= 0 || !validAutoPolicy(p) {
		return errcode.ErrBadRequest
	}
	if err := validateAutoRulesJSON(p.RulesJSON); err != nil {
		return err
	}
	if err := uc.autoRepo.UpdatePolicy(ctx, p); err != nil {
		logger.L.Errorw("update auto route policy failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) DeleteAutoRoutePolicy(ctx context.Context, id int64) error {
	if uc.autoRepo == nil || id <= 0 {
		return errcode.ErrBadRequest
	}
	if err := uc.autoRepo.DeletePolicy(ctx, id); err != nil {
		logger.L.Errorw("delete auto route policy failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) CreateAutoRouteCandidate(ctx context.Context, c *domainRouting.AutoRouteCandidate) error {
	if uc.autoRepo == nil || !validAutoCandidate(c) {
		return errcode.ErrBadRequest
	}
	if err := uc.autoRepo.CreateCandidate(ctx, c); err != nil {
		logger.L.Errorw("create auto route candidate failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) ListAutoRouteCandidates(ctx context.Context, policyID int64, activeOnly bool) ([]*domainRouting.AutoRouteCandidate, error) {
	if uc.autoRepo == nil || policyID <= 0 {
		return nil, errcode.ErrBadRequest
	}
	list, err := uc.autoRepo.ListCandidatesByPolicyID(ctx, policyID, activeOnly)
	if err != nil {
		logger.L.Errorw("list auto route candidates failed", "error", err)
		return nil, errcode.ErrInternal
	}
	return list, nil
}

func (uc *UseCase) UpdateAutoRouteCandidate(ctx context.Context, c *domainRouting.AutoRouteCandidate) error {
	if uc.autoRepo == nil || c.ID <= 0 || !validAutoCandidate(c) {
		return errcode.ErrBadRequest
	}
	if err := uc.autoRepo.UpdateCandidate(ctx, c); err != nil {
		logger.L.Errorw("update auto route candidate failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func (uc *UseCase) DeleteAutoRouteCandidate(ctx context.Context, id int64) error {
	if uc.autoRepo == nil || id <= 0 {
		return errcode.ErrBadRequest
	}
	if err := uc.autoRepo.DeleteCandidate(ctx, id); err != nil {
		logger.L.Errorw("delete auto route candidate failed", "error", err)
		return errcode.ErrInternal
	}
	return nil
}

func validAutoPolicy(p *domainRouting.AutoRoutePolicy) bool {
	if p == nil || p.VirtualModelID <= 0 || strings.TrimSpace(p.Name) == "" {
		return false
	}
	_, ok := allowedAutoStrategies[p.Strategy]
	return ok
}

func validAutoCandidate(c *domainRouting.AutoRouteCandidate) bool {
	if c == nil || c.PolicyID <= 0 || c.TargetModelID <= 0 {
		return false
	}
	return c.Weight >= 0 && c.Weight <= 1000 &&
		c.QualityScore >= 0 && c.QualityScore <= 100 &&
		c.CostBias >= 0 && c.CostBias <= 100 &&
		c.LatencyBias >= 0 && c.LatencyBias <= 100
}

func validateAutoRulesJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rules autoRules
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return errcode.ErrBadRequest
	}
	for _, tag := range rules.RequireCapabilities {
		if _, ok := allowedCapabilityTags[strings.ToLower(strings.TrimSpace(tag))]; !ok {
			return errcode.ErrBadRequest
		}
	}
	return nil
}
