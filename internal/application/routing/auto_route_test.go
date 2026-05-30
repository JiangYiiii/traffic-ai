package routing

import (
	"context"
	"errors"
	"testing"

	domainModel "github.com/trailyai/traffic-ai/internal/domain/model"
	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
)

type stubAutoRouteRepo struct {
	policiesByVirtualModelID map[int64]*domainRouting.AutoRoutePolicy
	candidatesByPolicyID     map[int64][]*domainRouting.AutoRouteCandidate
}

func (s *stubAutoRouteRepo) CreatePolicy(context.Context, *domainRouting.AutoRoutePolicy) error {
	return nil
}
func (s *stubAutoRouteRepo) FindPolicyByID(context.Context, int64) (*domainRouting.AutoRoutePolicy, error) {
	return nil, nil
}
func (s *stubAutoRouteRepo) FindActivePolicyByVirtualModelID(_ context.Context, virtualModelID int64) (*domainRouting.AutoRoutePolicy, error) {
	return s.policiesByVirtualModelID[virtualModelID], nil
}
func (s *stubAutoRouteRepo) ListPolicies(context.Context) ([]*domainRouting.AutoRoutePolicy, error) {
	return nil, nil
}
func (s *stubAutoRouteRepo) UpdatePolicy(context.Context, *domainRouting.AutoRoutePolicy) error {
	return nil
}
func (s *stubAutoRouteRepo) DeletePolicy(context.Context, int64) error {
	return nil
}
func (s *stubAutoRouteRepo) CreateCandidate(context.Context, *domainRouting.AutoRouteCandidate) error {
	return nil
}
func (s *stubAutoRouteRepo) FindCandidateByID(context.Context, int64) (*domainRouting.AutoRouteCandidate, error) {
	return nil, nil
}
func (s *stubAutoRouteRepo) ListCandidatesByPolicyID(_ context.Context, policyID int64, activeOnly bool) ([]*domainRouting.AutoRouteCandidate, error) {
	out := s.candidatesByPolicyID[policyID]
	if !activeOnly {
		return out, nil
	}
	filtered := make([]*domainRouting.AutoRouteCandidate, 0, len(out))
	for _, c := range out {
		if c.IsActive {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}
func (s *stubAutoRouteRepo) UpdateCandidate(context.Context, *domainRouting.AutoRouteCandidate) error {
	return nil
}
func (s *stubAutoRouteRepo) DeleteCandidate(context.Context, int64) error {
	return nil
}

func buildAutoFixture(t *testing.T) (*UseCase, *mockBreaker) {
	t.Helper()
	auto := &domainModel.Model{ID: 1, ModelName: "auto", IsActive: true, IsListed: true, IsVirtual: true, VirtualType: domainRouting.VirtualTypeAutoRoute}
	mini := &domainModel.Model{
		ID:                  2,
		ModelName:           "gpt-mini",
		Provider:            "openai",
		ModelType:           "chat",
		IsActive:            true,
		IsListed:            true,
		InputPrice:          100,
		OutputPrice:         200,
		ContextWindowTokens: 128000,
		CapabilityTags:      []string{"streaming", "coding"},
	}
	strong := &domainModel.Model{
		ID:                  3,
		ModelName:           "gpt-strong",
		Provider:            "openai",
		ModelType:           "chat",
		IsActive:            true,
		IsListed:            true,
		InputPrice:          500,
		OutputPrice:         1000,
		ContextWindowTokens: 128000,
		CapabilityTags:      []string{"streaming", "reasoning", "coding"},
	}
	tg := &stubTokenGroupRepo{accountIDsByGroupName: map[string][]int64{
		"default": {20, 30},
	}}
	mr := &stubModelRepo{
		byName: map[string]*domainModel.Model{"auto": auto, "gpt-mini": mini, "gpt-strong": strong},
		byID: map[int64]*domainModel.Model{
			1: auto,
			2: mini,
			3: strong,
		},
	}
	ar := &stubAccountRepo{activeByModelID: map[int64][]*domainModel.ModelAccount{
		2: {{ID: 20, ModelID: 2, Provider: "openai", Protocol: "openai", Weight: 1, IsActive: true, AuthType: "api_key", Credential: mustEncryptAuto(t, "sk-mini")}},
		3: {{ID: 30, ModelID: 3, Provider: "openai", Protocol: "messages", Weight: 1, IsActive: true, AuthType: "api_key", Credential: mustEncryptAuto(t, "sk-strong")}},
	}}
	autoRepo := &stubAutoRouteRepo{
		policiesByVirtualModelID: map[int64]*domainRouting.AutoRoutePolicy{
			1: {ID: 100, VirtualModelID: 1, Name: "AUTO", Strategy: domainRouting.AutoStrategyBalanced, RulesJSON: `{"require_capabilities":["streaming"]}`, IsActive: true, Version: 1},
		},
		candidatesByPolicyID: map[int64][]*domainRouting.AutoRouteCandidate{
			100: {
				{ID: 1001, PolicyID: 100, TargetModelID: 2, Priority: 10, Weight: 1, QualityScore: 60, IsActive: true},
				{ID: 1002, PolicyID: 100, TargetModelID: 3, Priority: 20, Weight: 1, QualityScore: 90, IsActive: true},
			},
		},
	}
	br := &mockBreaker{allow: map[int64]bool{}}
	uc := NewUseCase(tg, mr, ar, testAESKey, config.OAuthConfig{}, br, autoRepo)
	return uc, br
}

func mustEncryptAuto(t *testing.T, plain string) string {
	t.Helper()
	enc, err := crypto.EncryptAES(plain, testAESKey)
	if err != nil {
		t.Fatalf("EncryptAES: %v", err)
	}
	return enc
}

func TestSelectRoute_NonVirtualModelDelegatesToNormalRouting(t *testing.T) {
	uc, _, _, _, _ := buildFixture(t)
	got, err := uc.SelectRoute(context.Background(), domainRouting.RouteRequest{
		TokenGroup:     "default",
		RequestedModel: "gpt-4o",
		Protocol:       "openai",
	})
	if err != nil {
		t.Fatalf("SelectRoute: %v", err)
	}
	if got.IsAutoRoute {
		t.Fatal("non-virtual route marked as AUTO")
	}
	if got.RequestedModel != "gpt-4o" || got.ResolvedModel != "gpt-4o" {
		t.Fatalf("models = requested %q resolved %q", got.RequestedModel, got.ResolvedModel)
	}
}

func TestSelectRoute_AutoFiltersByTokenGroupAndProtocol(t *testing.T) {
	uc, _ := buildAutoFixture(t)
	got, err := uc.SelectRoute(context.Background(), domainRouting.RouteRequest{
		TokenGroup:      "default",
		RequestedModel:  "auto",
		Protocol:        "openai",
		EstimatedTokens: 1000,
		Stream:          true,
	})
	if err != nil {
		t.Fatalf("SelectRoute: %v", err)
	}
	if !got.IsAutoRoute {
		t.Fatal("expected AUTO route")
	}
	if got.PolicyID != 100 || got.RequestedModel != "auto" || got.ResolvedModel != "gpt-mini" {
		t.Fatalf("unexpected route: %#v", got)
	}
	if got.Account.ID != 20 {
		t.Fatalf("account ID = %d, want 20", got.Account.ID)
	}
}

func TestSelectRoute_AutoSkipsCircuitOpenAccounts(t *testing.T) {
	uc, br := buildAutoFixture(t)
	br.allow[20] = false
	_, err := uc.SelectRoute(context.Background(), domainRouting.RouteRequest{
		TokenGroup:      "default",
		RequestedModel:  "auto",
		Protocol:        "openai",
		EstimatedTokens: 1000,
		Stream:          true,
	})
	if !errors.Is(err, errcode.ErrNoAvailableRoute) {
		t.Fatalf("expected ErrNoAvailableRoute, got %v", err)
	}
}
