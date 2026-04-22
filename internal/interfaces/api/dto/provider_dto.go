package dto

// DiscoverProviderModelsReq 统一查询体：先选商家 provider、接入方式 auth_type，再填 credential（及可选 base_url）。
type DiscoverProviderModelsReq struct {
	Provider   string `json:"provider" binding:"required"`
	AuthType   string `json:"auth_type" binding:"required"`
	Credential string `json:"credential" binding:"required"`
	BaseURL    string `json:"base_url"`
}
