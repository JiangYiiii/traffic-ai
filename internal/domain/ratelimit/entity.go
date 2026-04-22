package ratelimit

import "time"

type Scope string

const (
	ScopeGlobal Scope = "global"
	ScopeUser   Scope = "user"
	ScopeAPIKey Scope = "api_key"
	ScopeModel  Scope = "model"
)

// CheckOrder 定义多级限流的检查顺序。
var CheckOrder = []Scope{ScopeGlobal, ScopeUser, ScopeAPIKey, ScopeModel}

type RateLimitRule struct {
	ID            int64
	Name          string
	Scope         Scope
	ScopeValue    string
	MaxRPM        int
	MaxTPM        int
	MaxConcurrent int
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// HasRPMLimit 是否配置了 RPM 限制。
func (r *RateLimitRule) HasRPMLimit() bool { return r.MaxRPM > 0 }

// HasConcurrentLimit 是否配置了并发限制。
func (r *RateLimitRule) HasConcurrentLimit() bool { return r.MaxConcurrent > 0 }

// HasTPMLimit 是否配置了 TPM（Tokens Per Minute）限制。
func (r *RateLimitRule) HasTPMLimit() bool { return r.MaxTPM > 0 }
