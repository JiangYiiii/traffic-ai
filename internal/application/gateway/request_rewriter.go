// Package gateway — 上游请求体的小型重写工具。
//
// 只做"对计费/日志统计正确性必不可少"的字段注入，不改业务语义：
// 当前仅处理 OpenAI `/v1/chat/completions` 流式请求。OpenAI SSE 默认不回 usage，
// 客户端需显式传 `stream_options.include_usage=true` 才会在末帧拿到 token 统计。
// 网关侧如果透传原始 body，多数客户端会漏掉这个选项，导致 usage_logs 里 token=0、
// 计费也跟着是 0。这里在"客户端没显式选择"时兜底打开，保障统计与计费准确。
package gateway

import "encoding/json"

// injectIncludeUsageForOpenAIStream 为 OpenAI /chat/completions 流式请求补齐
// stream_options.include_usage=true。若 body 不是合法 JSON 对象、或客户端已显式
// 设置 include_usage（不管 true/false），都原样返回，不修改用户意图。
//
// 保留其它字段与值原貌（包括未知字段），最大化兼容上游扩展。
func injectIncludeUsageForOpenAIStream(body []byte) []byte {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return body
	}

	var opts map[string]json.RawMessage
	if raw, ok := root["stream_options"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &opts); err != nil {
			// stream_options 不是对象（异常但合法 JSON），不敢改，原样返回
			return body
		}
	}
	if opts == nil {
		opts = make(map[string]json.RawMessage, 1)
	}

	if _, exists := opts["include_usage"]; exists {
		return body
	}

	opts["include_usage"] = json.RawMessage("true")
	newOpts, err := json.Marshal(opts)
	if err != nil {
		return body
	}
	root["stream_options"] = newOpts

	newBody, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return newBody
}
