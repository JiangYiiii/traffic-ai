// Package gateway — token usage 解析按协议分发。
//
// 上游响应的 usage 字段分三种家族：
//   - OpenAI 家族（openai / responses / embeddings / speech）：`usage.{prompt_tokens,completion_tokens,total_tokens,
//     completion_tokens_details.reasoning_tokens}`；Responses API 还会额外出现 `usage.{input_tokens,output_tokens}`，
//     同一个 lenientUsage 结构即可同时兼容。
//   - Anthropic：`usage.{input_tokens,output_tokens,cache_creation_input_tokens,cache_read_input_tokens}`；
//     流式 SSE 里输入 token 来自 `message_start.message.usage`，输出 token 在 `message_delta.usage`（覆盖式累计）。
//   - Gemini：`usageMetadata.{promptTokenCount,candidatesTokenCount,totalTokenCount,thoughtsTokenCount,cachedContentTokenCount}`；
//     流式最终以最后一帧为准。
package gateway

import "encoding/json"

// lenientUsage 同时兼容 OpenAI 与 Anthropic 的 `usage` 结构：
// OpenAI 用 prompt_tokens/completion_tokens，Anthropic 用 input_tokens/output_tokens。
type lenientUsage struct {
	// OpenAI style
	PromptTokens            int                      `json:"prompt_tokens,omitempty"`
	CompletionTokens        int                      `json:"completion_tokens,omitempty"`
	CompletionTokensDetails *completionTokenDetails `json:"completion_tokens_details,omitempty"`
	// Anthropic / OpenAI Responses style
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	// 通用合计
	TotalTokens int `json:"total_tokens,omitempty"`
	// Anthropic 缓存字段（OpenAI cached_prompt 也兼容到这里）
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type completionTokenDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// geminiUsageMetadata 对应 Gemini generateContent 的 usageMetadata。
type geminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}

// streamUsageState 流式解析时按协议累计的 token 状态。
// Anthropic 流式会分 message_start / message_delta 两种事件逐步补齐，因此必须持久化到整段流结束。
type streamUsageState struct {
	InputTokens         int
	OutputTokens        int
	TotalTokens         int
	ReasoningTokens     int
	CacheCreationTokens int
	CacheReadTokens     int
}

// toProxyResult 把累计态复制到 ProxyResult，TotalTokens 若上游未给则自动求和兜底。
func (s *streamUsageState) toProxyResult() ProxyResult {
	total := s.TotalTokens
	if total == 0 {
		total = s.InputTokens + s.OutputTokens
	}
	return ProxyResult{
		InputTokens:         s.InputTokens,
		OutputTokens:        s.OutputTokens,
		TotalTokens:         total,
		ReasoningTokens:     s.ReasoningTokens,
		CacheCreationTokens: s.CacheCreationTokens,
		CacheReadTokens:     s.CacheReadTokens,
	}
}

// extractUsageFromBody 从完整（非流式）响应体里解析 token usage。
// protocol 的取值与 genericProxy / ChatCompletions 传入保持一致（"openai"/"responses"/"anthropic"/"gemini"/
// "embeddings"/"speech"/""）。
func extractUsageFromBody(protocol string, body []byte) ProxyResult {
	switch protocol {
	case "gemini":
		var r struct {
			UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
		}
		if err := json.Unmarshal(body, &r); err != nil || r.UsageMetadata == nil {
			return ProxyResult{}
		}
		return geminiMetaToResult(r.UsageMetadata)
	default:
		// OpenAI / Anthropic / Responses / Embeddings / Speech 全部走 `usage` 包装
		var r struct {
			Usage *lenientUsage `json:"usage"`
		}
		if err := json.Unmarshal(body, &r); err != nil || r.Usage == nil {
			return ProxyResult{}
		}
		res := lenientUsageToResult(r.Usage)
		if res.TotalTokens == 0 {
			res.TotalTokens = res.InputTokens + res.OutputTokens
		}
		return res
	}
}

// updateStreamUsage 在 SSE data 行上增量更新 state。传入的 dataLine 已去掉 "data: " 前缀。
func updateStreamUsage(protocol string, dataLine string, state *streamUsageState) {
	if dataLine == "" || dataLine == "[DONE]" {
		return
	}
	raw := []byte(dataLine)
	switch protocol {
	case "anthropic":
		var evt struct {
			Type    string `json:"type"`
			Message *struct {
				Usage *lenientUsage `json:"usage"`
			} `json:"message"`
			Usage *lenientUsage `json:"usage"`
		}
		if err := json.Unmarshal(raw, &evt); err != nil {
			return
		}
		switch evt.Type {
		case "message_start":
			if evt.Message != nil && evt.Message.Usage != nil {
				u := evt.Message.Usage
				if u.InputTokens > 0 {
					state.InputTokens = u.InputTokens
				}
				if u.OutputTokens > 0 {
					state.OutputTokens = u.OutputTokens
				}
				if u.CacheCreationInputTokens > 0 {
					state.CacheCreationTokens = u.CacheCreationInputTokens
				}
				if u.CacheReadInputTokens > 0 {
					state.CacheReadTokens = u.CacheReadInputTokens
				}
			}
		case "message_delta":
			// message_delta 里 usage.output_tokens 是当前累计输出 token（覆盖式）
			if evt.Usage != nil {
				if evt.Usage.InputTokens > 0 {
					state.InputTokens = evt.Usage.InputTokens
				}
				if evt.Usage.OutputTokens > 0 {
					state.OutputTokens = evt.Usage.OutputTokens
				}
				if evt.Usage.CacheCreationInputTokens > 0 {
					state.CacheCreationTokens = evt.Usage.CacheCreationInputTokens
				}
				if evt.Usage.CacheReadInputTokens > 0 {
					state.CacheReadTokens = evt.Usage.CacheReadInputTokens
				}
			}
		}
	case "gemini":
		var r struct {
			UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
		}
		if err := json.Unmarshal(raw, &r); err != nil || r.UsageMetadata == nil {
			return
		}
		applyGeminiMetaToState(r.UsageMetadata, state)
	default:
		// OpenAI 兼容 SSE：chunk 自身通常无 usage，只有最后一条 chunk 里会带；
		// 对于 Responses 流式亦走此分支（字段名也由 lenientUsage 覆盖）。
		var r struct {
			Usage *lenientUsage `json:"usage"`
		}
		if err := json.Unmarshal(raw, &r); err != nil || r.Usage == nil {
			return
		}
		applyLenientUsageToState(r.Usage, state)
	}
}

func applyLenientUsageToState(u *lenientUsage, state *streamUsageState) {
	// OpenAI style
	if u.PromptTokens > 0 {
		state.InputTokens = u.PromptTokens
	}
	if u.CompletionTokens > 0 {
		state.OutputTokens = u.CompletionTokens
	}
	// Anthropic / Responses style
	if u.InputTokens > 0 {
		state.InputTokens = u.InputTokens
	}
	if u.OutputTokens > 0 {
		state.OutputTokens = u.OutputTokens
	}
	if u.TotalTokens > 0 {
		state.TotalTokens = u.TotalTokens
	}
	if u.CompletionTokensDetails != nil && u.CompletionTokensDetails.ReasoningTokens > 0 {
		state.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	if u.CacheCreationInputTokens > 0 {
		state.CacheCreationTokens = u.CacheCreationInputTokens
	}
	if u.CacheReadInputTokens > 0 {
		state.CacheReadTokens = u.CacheReadInputTokens
	}
}

func applyGeminiMetaToState(u *geminiUsageMetadata, state *streamUsageState) {
	if u.PromptTokenCount > 0 {
		state.InputTokens = u.PromptTokenCount
	}
	if u.CandidatesTokenCount > 0 {
		state.OutputTokens = u.CandidatesTokenCount
	}
	if u.TotalTokenCount > 0 {
		state.TotalTokens = u.TotalTokenCount
	}
	if u.ThoughtsTokenCount > 0 {
		state.ReasoningTokens = u.ThoughtsTokenCount
	}
	if u.CachedContentTokenCount > 0 {
		state.CacheReadTokens = u.CachedContentTokenCount
	}
}

func lenientUsageToResult(u *lenientUsage) ProxyResult {
	r := ProxyResult{}
	if u.PromptTokens > 0 {
		r.InputTokens = u.PromptTokens
	}
	if u.CompletionTokens > 0 {
		r.OutputTokens = u.CompletionTokens
	}
	if u.InputTokens > 0 {
		r.InputTokens = u.InputTokens
	}
	if u.OutputTokens > 0 {
		r.OutputTokens = u.OutputTokens
	}
	if u.TotalTokens > 0 {
		r.TotalTokens = u.TotalTokens
	}
	if u.CompletionTokensDetails != nil {
		r.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	r.CacheCreationTokens = u.CacheCreationInputTokens
	r.CacheReadTokens = u.CacheReadInputTokens
	return r
}

func geminiMetaToResult(u *geminiUsageMetadata) ProxyResult {
	total := u.TotalTokenCount
	if total == 0 {
		total = u.PromptTokenCount + u.CandidatesTokenCount
	}
	return ProxyResult{
		InputTokens:     u.PromptTokenCount,
		OutputTokens:    u.CandidatesTokenCount,
		TotalTokens:     total,
		ReasoningTokens: u.ThoughtsTokenCount,
		CacheReadTokens: u.CachedContentTokenCount,
	}
}
