// Package provider 定义管理台可接入的「商家 / 协议」元数据，用于统一拉取远端模型列表。
package provider

// AuthType 与前端「接入方式」一致；查询远端列表时均映射为具体 HTTP 头。
const (
	AuthAPIKey       = "api_key"
	AuthOAuthBearer  = "oauth_bearer"
)

// Kind 决定拉取模型列表的 HTTP 形态（同一 Kind 下查询方式一致，仅 base URL / 鉴权不同）。
type Kind string

const (
	KindOpenAIv1Models    Kind = "openai_v1_models"    // GET {base}/models
	KindAnthropicv1Models Kind = "anthropic_v1_models" // GET {base}/models + x-api-key
	KindGeminiList        Kind = "gemini_v1beta_list"  // GET .../v1beta/models?key=
	KindManualOnly        Kind = "manual_only"         // 不提供列表接口，仅手填
)

// Definition 静态商家目录（非 DB）。
type Definition struct {
	ID              string
	DisplayName     string
	VendorGroup     string   // 厂商分组，用于前端 <optgroup> 展示
	AuthTypes       []string // 允许的 auth_type，如 api_key、oauth_bearer
	DefaultBaseURL  string   // 不含末尾 /
	RequireBaseURL  bool     // true 时必须传 base_url（自定义兼容网关）
	Kind            Kind
	ProviderTag     string // 写入 models.provider 时的建议值，与 ID 一致即可
	OAuthConfigKey  string // 映射到 config.yaml 中 oauth.providers 的 key，为空则该商家不支持 OAuth
}

// Catalog 内置商家列表：先选商家 → 再选接入方式 → 再填密钥 → 用统一接口 discover。
var Catalog = []Definition{
	// ---- 海外 ----
	{
		ID: "openai", DisplayName: "OpenAI",
		VendorGroup:    "OpenAI",
		AuthTypes:      []string{AuthAPIKey, AuthOAuthBearer},
		DefaultBaseURL: "https://api.openai.com/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "openai",
		OAuthConfigKey: "openai",
	},
	{
		ID: "anthropic", DisplayName: "Anthropic",
		VendorGroup:    "Anthropic",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://api.anthropic.com/v1",
		Kind:           KindAnthropicv1Models,
		ProviderTag:    "anthropic",
	},
	{
		ID: "google_gemini", DisplayName: "Google AI Studio（Gemini API Key）",
		VendorGroup:    "Google",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta",
		Kind:           KindGeminiList,
		ProviderTag:    "google",
	},
	// ---- 国内 ----
	{
		ID: "deepseek", DisplayName: "DeepSeek",
		VendorGroup:    "DeepSeek",
		AuthTypes:      []string{AuthAPIKey, AuthOAuthBearer},
		DefaultBaseURL: "https://api.deepseek.com/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "deepseek",
	},
	{
		ID: "moonshot", DisplayName: "Moonshot（Kimi）",
		VendorGroup:    "月之暗面",
		AuthTypes:      []string{AuthAPIKey, AuthOAuthBearer},
		DefaultBaseURL: "https://api.moonshot.cn/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "moonshot",
	},
	{
		ID: "qwen", DisplayName: "通义千问",
		VendorGroup:    "阿里",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "qwen",
	},
	{
		ID: "zhipu", DisplayName: "智谱 ChatGLM",
		VendorGroup:    "智谱 AI",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "zhipu",
	},
	{
		ID: "doubao", DisplayName: "豆包（火山方舟）",
		VendorGroup:    "字节跳动",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "doubao",
	},
	{
		ID: "ernie", DisplayName: "文心一言",
		VendorGroup:    "百度",
		AuthTypes:      []string{AuthAPIKey, AuthOAuthBearer},
		DefaultBaseURL: "https://qianfan.baidubce.com/v2",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "ernie",
	},
	{
		ID: "hunyuan", DisplayName: "腾讯混元",
		VendorGroup:    "腾讯",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://api.hunyuan.cloud.tencent.com/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "hunyuan",
	},
	{
		ID: "spark", DisplayName: "讯飞星火",
		VendorGroup:    "科大讯飞",
		AuthTypes:      []string{AuthAPIKey, AuthOAuthBearer},
		DefaultBaseURL: "https://spark-api-open.xf-yun.com/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "spark",
	},
	{
		ID: "baichuan", DisplayName: "百川",
		VendorGroup:    "百川智能",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://api.baichuan-ai.com/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "baichuan",
	},
	{
		ID: "yi", DisplayName: "零一万物（Yi）",
		VendorGroup:    "零一万物",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://api.lingyiwanwu.com/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "yi",
	},
	{
		ID: "minimax", DisplayName: "MiniMax",
		VendorGroup:    "MiniMax",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://api.minimax.chat/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "minimax",
	},
	{
		ID: "stepfun", DisplayName: "阶跃星辰",
		VendorGroup:    "阶跃星辰",
		AuthTypes:      []string{AuthAPIKey},
		DefaultBaseURL: "https://api.stepfun.com/v1",
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "stepfun",
	},
	// ---- 通用 ----
	{
		ID: "openai_compatible", DisplayName: "OpenAI 兼容（自定义 Base URL）",
		VendorGroup:    "通用",
		AuthTypes:      []string{AuthAPIKey, AuthOAuthBearer},
		RequireBaseURL: true,
		Kind:           KindOpenAIv1Models,
		ProviderTag:    "openai_compatible",
	},
	{
		ID: "manual", DisplayName: "仅手动录入（不拉取列表）",
		VendorGroup:    "通用",
		AuthTypes:      []string{AuthAPIKey, AuthOAuthBearer},
		RequireBaseURL: true,
		Kind:           KindManualOnly,
		ProviderTag:    "manual",
	},
}

// ByID 返回目录项；不存在则 ok=false。
func ByID(id string) (def Definition, ok bool) {
	for _, d := range Catalog {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}
