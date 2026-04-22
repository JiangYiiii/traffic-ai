package model

import "time"

type BillingType string

const (
	BillingPerToken   BillingType = "per_token"
	BillingPerRequest BillingType = "per_request"
)

type Model struct {
	ID                int64
	ModelName         string
	Provider          string // 向后兼容的聚合展示字段，真实 provider 存在其 model_accounts 上
	ModelType         string
	BillingType       BillingType
	InputPrice        int64
	OutputPrice       int64
	ReasoningPrice    int64
	PerRequestPrice   int64
	IsActive          bool
	IsListed          bool // 是否上架展示给用户
	LastTestPassed    *bool
	LastTestAt        *time.Time
	LastTestLatencyMs *int
	LastTestError     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ModelAccount 一个模型下的账号：代表一条"怎么去第三方调这个模型"的完整配置。
//
// 业务语义：
//   - 用户只选 Model，路由层在该 Model 的 Account 池里挑一个健康账号
//   - 每个账号保存一种完整的连接方式：protocol + endpoint + credential + auth_type
//   - Model ←1:N→ ModelAccount；账号不跨模型共享，同一把 Key 若要同时服务 gpt-4o / gpt-4o-mini，
//     需在两个模型下各建一个账号（设计上避免跨模型的隐式耦合）
type ModelAccount struct {
	ID                int64
	ModelID           int64
	Provider          string // openai | anthropic | google | azure | ...，冗余存储以支持跨账号聚合
	Name              string // 账号显示名，如 "OpenAI-主力号-us-east"
	Endpoint          string
	Credential        string     // AES-256 encrypted in DB, plaintext in memory after decryption
	AuthType          string     // "api_key" | "oauth_authorization_code"
	RefreshToken      string     // AES-256 encrypted, only for OAuth
	TokenExpiresAt    *time.Time // access_token 到期时间, nil for api_key
	Protocol          string
	Weight            int
	IsActive          bool
	TimeoutSec        int
	LastTestPassed    *bool
	LastTestAt        *time.Time
	LastTestLatencyMs *int
	LastTestError     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Status 把 IsActive 映射为对外展示的字符串状态。
func (a *ModelAccount) Status() string {
	if a.IsActive {
		return ModelAccountStatusOnline
	}
	return ModelAccountStatusOffline
}

const (
	ModelAccountStatusOnline  = "online"
	ModelAccountStatusOffline = "offline"
)
