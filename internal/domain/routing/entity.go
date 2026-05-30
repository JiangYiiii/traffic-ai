package routing

import (
	"time"

	"github.com/trailyai/traffic-ai/internal/domain/model"
)

type TokenGroup struct {
	ID          int64
	Name        string
	Description string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TokenGroupModelAccount 表示 token 分组与模型账号的多对多关系。
type TokenGroupModelAccount struct {
	ID             int64
	TokenGroupID   int64
	ModelAccountID int64
}

const (
	VirtualTypeAutoRoute = "auto_route"

	AutoStrategyBalanced  = "balanced"
	AutoStrategyFast      = "fast"
	AutoStrategyCheap     = "cheap"
	AutoStrategyQuality   = "quality"
	AutoStrategyCoding    = "coding"
	AutoStrategyReasoning = "reasoning"
)

type AutoRoutePolicy struct {
	ID             int64
	VirtualModelID int64
	Name           string
	Strategy       string
	RulesJSON      string
	IsActive       bool
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AutoRouteCandidate struct {
	ID                      int64
	PolicyID                int64
	TargetModelID           int64
	Priority                int
	Weight                  int
	MinRequestContextTokens int
	QualityScore            int
	CostBias                int
	LatencyBias             int
	IsActive                bool
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type RequestFeatures struct {
	HasCode        bool
	HasLongContext bool
	WantsReasoning bool
	WantsJSON      bool
}

type RouteRequest struct {
	TokenGroup        string
	RequestedModel    string
	Protocol          string
	UserID            int64
	APIKeyID          int64
	EstimatedTokens   int
	Stream            bool
	ReasoningEffort   string
	RequestFeatures   RequestFeatures
	ExcludeAccountIDs []int64
	ExcludeModelIDs   []int64
}

type RouteDecision struct {
	Account        *model.ModelAccount
	Model          *model.Model
	RequestedModel string
	ResolvedModel  string
	IsAutoRoute    bool
	PolicyID       int64
	Mode           string
	Score          int
	Reason         string
}
