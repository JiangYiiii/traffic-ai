// Package gateway — 上游请求体的小型重写工具。
//
// 只做「对计费/日志统计正确性」或「与上游/Azure 约定对齐」的小型改写：
// 当前处理 OpenAI `/v1/chat/completions` 流式请求的 usage 注入，以及 Azure deployment
// 形态下 `/images/generations` 去掉冗余 `model` 字段。OpenAI SSE 默认不回 usage，
// 客户端需显式传 `stream_options.include_usage=true` 才会在末帧拿到 token 统计。
// 网关侧如果透传原始 body，多数客户端会漏掉这个选项，导致 usage_logs 里 token=0、
// 计费也跟着是 0。这里在"客户端没显式选择"时兜底打开，保障统计与计费准确。
package gateway

import (
	"encoding/json"
	"strings"
)

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

// stripTopLevelJSONKey 从顶层 JSON 对象中删除指定键；非法 JSON、非对象、或键不存在时原样返回。
//
// Azure OpenAI 兼容上游常见形态为「…/openai/deployments/{name}?api-version=…」，模型已由
// URL 中的 deployment 指定。此时若仍透传 OpenAI 官方 body 里的 `model` 字段，部分上游会
// 返回 400「The requested operation is unsupported」或行为异常；管理端 Playground 生图
//（playgroundImage）发往 /images/generations 的 JSON 不含 model，与上游约定一致。
func stripTopLevelJSONKey(body []byte, key string) []byte {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return body
	}
	if _, ok := root[key]; !ok {
		return body
	}
	delete(root, key)
	out, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return out
}

// azureOpenAIDeploymentEndpoint 是否为「deployment 写在 URL 路径」的 Azure OpenAI 兼容 endpoint。
func azureOpenAIDeploymentEndpoint(endpoint string) bool {
	e := strings.ToLower(strings.TrimSpace(endpoint))
	return strings.Contains(e, "/openai/deployments/")
}
