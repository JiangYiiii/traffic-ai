package model

import (
	"context"

	"github.com/trailyai/traffic-ai/internal/domain/provider"
	"github.com/trailyai/traffic-ai/internal/infrastructure/provider/remotediscovery"
)

// DiscoverOutcome 管理台「拉取远端模型」统一结果：失败时 FetchFailed=true，前端仍可走原有手动创建模型/线路流程。
type DiscoverOutcome struct {
	Models      []DiscoverModelItem `json:"models"`
	FetchFailed bool                `json:"fetch_failed"`
	Message     string              `json:"message,omitempty"`
	ManualOK    bool                `json:"manual_ok"` // 恒为 true，提示可手填
	Provider    string              `json:"provider,omitempty"`
}

// DiscoverModelItem 单条远端模型 id（与 OpenAI /v1/models 的 id 对齐）。
type DiscoverModelItem struct {
	ID string `json:"id"`
}

// DiscoverRemoteModels 先选商家与接入方式并填密钥后调用；与具体商家无关的统一入口。
func (uc *UseCase) DiscoverRemoteModels(ctx context.Context, providerID, authType, credential, baseURL string) DiscoverOutcome {
	out := DiscoverOutcome{ManualOK: true, Provider: providerID}
	def, ok := provider.ByID(providerID)
	if !ok {
		out.FetchFailed = true
		out.Message = "未知商家"
		return out
	}
	valid := false
	for _, a := range def.AuthTypes {
		if a == authType {
			valid = true
			break
		}
	}
	if !valid {
		out.FetchFailed = true
		out.Message = "该商家不支持此接入方式"
		return out
	}
	if def.RequireBaseURL && baseURL == "" {
		out.FetchFailed = true
		out.Message = "请填写 Base URL"
		return out
	}

	res := remotediscovery.Discover(ctx, def, authType, credential, baseURL)
	if res.Err != nil {
		out.FetchFailed = true
		out.Message = res.Err.Error()
		return out
	}
	out.Models = make([]DiscoverModelItem, 0, len(res.Models))
	for _, m := range res.Models {
		out.Models = append(out.Models, DiscoverModelItem{ID: m.ID})
	}
	return out
}
