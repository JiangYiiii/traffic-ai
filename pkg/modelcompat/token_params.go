// Package modelcompat 提供跨模型 API 兼容性适配工具。
//
// 背景：OpenAI 2024年12月起，新一代模型（GPT-4o 2024-11-20+、o1、o3、GPT-5 系列）
// 弃用 max_tokens 参数，改用 max_completion_tokens。旧模型仍需 max_tokens。
// 本包提供统一判断逻辑，避免在业务代码中散布字符串前缀匹配。
package modelcompat

import "strings"

// UsesMaxCompletionTokens 判断给定模型名是否使用新参数 max_completion_tokens。
//
// 规则（基于 OpenAI API 2024-12-01 文档）：
//   - GPT-4o 2024-11-20 及之后的快照版本
//   - o1 系列（o1-preview, o1, o1-mini）
//   - o3 系列（o3-mini, o3）
//   - GPT-5 系列（gpt-5.*, gpt-5-*)
//   - 其他厂商暂不涉及（Anthropic/Google/国产模型均不支持此参数）
//
// 参数 modelName：完整模型名称，如 "gpt-4o-2024-11-20"、"o1-preview"。
// 返回值：true 表示需要用 max_completion_tokens；false 表示用 max_tokens。
func UsesMaxCompletionTokens(modelName string) bool {
	m := strings.ToLower(modelName)

	// GPT-4o 2024-11-20+ 快照（包括 2025+ 年份）
	if strings.HasPrefix(m, "gpt-4o-") && len(m) >= len("gpt-4o-2024-11-20") {
		// 提取日期部分 YYYY-MM-DD，比较是否 >= 2024-11-20
		dateStr := m[len("gpt-4o-"):]  // "2024-11-20", "2024-08-06", "2025-01-15" 等
		// 字典序比较适用于 ISO 日期格式 YYYY-MM-DD
		if dateStr >= "2024-11-20" {
			return true
		}
	}

	// o1 系列：o1-preview, o1-mini, o1（但不包括 o1nce 之类的非官方命名）
	if strings.HasPrefix(m, "o1-") || m == "o1" {
		return true
	}

	// o3 系列：o3-mini, o3
	if strings.HasPrefix(m, "o3-") || m == "o3" {
		return true
	}

	// GPT-5 系列：gpt-5.4, gpt-5-turbo 等（但不包括 gpt-50, gpt-500）
	if strings.HasPrefix(m, "gpt-5") {
		// 确保后面不是数字（避免匹配 gpt-50, gpt-500）
		if len(m) == len("gpt-5") {
			return true // 恰好是 "gpt-5"
		}
		nextChar := m[len("gpt-5")]
		// gpt-5 后面是 . 或 - 或其他非数字字符
		if nextChar < '0' || nextChar > '9' {
			return true
		}
	}

	return false
}

// TokenLimitParamName 返回给定模型应使用的 token 限制参数名。
// 便捷封装，避免业务代码重复写 if-else。
func TokenLimitParamName(modelName string) string {
	if UsesMaxCompletionTokens(modelName) {
		return "max_completion_tokens"
	}
	return "max_tokens"
}
