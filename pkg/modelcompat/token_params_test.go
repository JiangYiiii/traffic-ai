package modelcompat

import "testing"

func TestUsesMaxCompletionTokens(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		want      bool
	}{
		// 新模型 - 应使用 max_completion_tokens
		{"GPT-4o 2024-11-20", "gpt-4o-2024-11-20", true},
		{"GPT-4o 2024-12-01", "gpt-4o-2024-12-01", true},
		{"GPT-4o 2025-01-15", "gpt-4o-2025-01-15", true},
		{"o1-preview", "o1-preview", true},
		{"o1-mini", "o1-mini", true},
		{"o1", "o1", true},
		{"o3-mini", "o3-mini", true},
		{"o3", "o3", true},
		{"GPT-5.4", "gpt-5.4", true},
		{"GPT-5", "gpt-5", true},
		{"GPT-5-turbo", "gpt-5-turbo", true},

		// 大小写不敏感
		{"GPT-4o uppercase", "GPT-4o-2024-11-20", true},
		{"O1-PREVIEW uppercase", "O1-PREVIEW", true},

		// 旧模型 - 应使用 max_tokens
		{"GPT-4o 2024-08-06", "gpt-4o-2024-08-06", false},
		{"GPT-4o 2024-11-01", "gpt-4o-2024-11-01", false},
		{"GPT-4o 2024-10-31", "gpt-4o-2024-10-31", false},
		{"gpt-4", "gpt-4", false},
		{"gpt-4-turbo", "gpt-4-turbo", false},
		{"gpt-3.5-turbo", "gpt-3.5-turbo", false},
		{"gpt-4-32k", "gpt-4-32k", false},

		// 其他厂商模型 - 使用 max_tokens
		{"Claude", "claude-3-5-sonnet-20241022", false},
		{"Gemini", "gemini-2.0-flash-exp", false},
		{"DeepSeek", "deepseek-chat", false},
		{"Qwen", "qwen-max", false},

		// 边界情况
		{"o1nce (非官方)", "o1nce", false}, // 不是标准 o1 系列
		{"gpt-50 (假设)", "gpt-50", false}, // gpt-5 后面必须是非数字或结束
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UsesMaxCompletionTokens(tt.modelName)
			if got != tt.want {
				t.Errorf("UsesMaxCompletionTokens(%q) = %v, want %v", tt.modelName, got, tt.want)
			}
		})
	}
}

func TestTokenLimitParamName(t *testing.T) {
	tests := []struct {
		modelName string
		want      string
	}{
		{"gpt-5.4", "max_completion_tokens"},
		{"o1", "max_completion_tokens"},
		{"gpt-4o-2024-11-20", "max_completion_tokens"},
		{"gpt-4", "max_tokens"},
		{"claude-3-5-sonnet-20241022", "max_tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			got := TokenLimitParamName(tt.modelName)
			if got != tt.want {
				t.Errorf("TokenLimitParamName(%q) = %q, want %q", tt.modelName, got, tt.want)
			}
		})
	}
}
