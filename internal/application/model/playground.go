package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domain "github.com/trailyai/traffic-ai/internal/domain/model"
	"github.com/trailyai/traffic-ai/internal/pkg/upstreamurl"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
	"github.com/trailyai/traffic-ai/pkg/modelcompat"
)

// PlaygroundResult Playground 调试结果。
type PlaygroundResult struct {
	Success        bool   `json:"success"`
	LatencyMs      int    `json:"latency_ms"`
	Model          string `json:"model"`
	Account        string `json:"account"`
	Assistant      string `json:"assistant,omitempty"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Error          string `json:"error,omitempty"`
	RawBodySnippet string `json:"raw_body_snippet,omitempty"`
}

// PlaygroundChat 向模型对应账号发送 chat/completions，返回助手文本或错误摘要。
func (uc *UseCase) PlaygroundChat(ctx context.Context, modelID int64, messages []map[string]string, maxTokens int) (*PlaygroundResult, error) {
	m, err := uc.modelRepo.FindByID(ctx, modelID)
	if err != nil {
		logger.L.Errorw("playground: find model failed", "error", err)
		return nil, errcode.ErrInternal
	}
	if m == nil {
		return nil, errcode.ErrModelNotFound
	}
	accounts, err := uc.accountRepo.ListByModelID(ctx, modelID)
	if err != nil {
		logger.L.Errorw("playground: list model accounts failed", "error", err)
		return nil, errcode.ErrInternal
	}
	var target *domain.ModelAccount
	for _, a := range accounts {
		if a.IsActive {
			target = a
			break
		}
	}
	if target == nil {
		return nil, errcode.ErrNoAvailableRoute
	}
	plain, err := crypto.DecryptAES(target.Credential, uc.aesKey)
	if err != nil {
		logger.L.Errorw("playground: decrypt credential failed", "error", err)
		return &PlaygroundResult{
			Success: false,
			Model:   m.ModelName,
			Account: accountLabel(target),
			Error:   "账号密钥解密失败：请编辑该账号并重新保存 API Key",
		}, nil
	}
	if len(messages) == 0 {
		messages = []map[string]string{{"role": "user", "content": "Say hello in one short sentence."}}
	}
	if maxTokens <= 0 {
		maxTokens = 256
	}
	if maxTokens > 4096 {
		maxTokens = 4096
	}

	// 根据模型选择正确的 token 限制参数名
	// GPT-4o 2024-11-20+、o1、o3、GPT-5 使用 max_completion_tokens
	// 其他模型使用 max_tokens
	payload := map[string]interface{}{
		"model":       m.ModelName,
		"messages":    messages,
		"temperature": 0.3,
	}
	tokenParamName := modelcompat.TokenLimitParamName(m.ModelName)
	payload[tokenParamName] = maxTokens

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return &PlaygroundResult{Success: false, Model: m.ModelName, Account: accountLabel(target), Error: fmt.Sprintf("build json: %v", err)}, nil
	}

	url := upstreamurl.JoinPath(target.Endpoint, "/chat/completions")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return &PlaygroundResult{Success: false, Model: m.ModelName, Account: accountLabel(target), Error: fmt.Sprintf("build request: %v", err)}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+plain)

	client := &http.Client{Timeout: 60 * time.Second}
	start := time.Now()
	resp, err := client.Do(httpReq)
	latency := time.Since(start)
	out := &PlaygroundResult{
		Model:     m.ModelName,
		Account:   accountLabel(target),
		LatencyMs: int(latency.Milliseconds()),
	}
	if err != nil {
		out.Error = fmt.Sprintf("request failed: %v", err)
		return out, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		out.RawBodySnippet = trimSnippet(string(body), 2000)
		return out, nil
	}
	assistant, perr := parseAssistantContent(body)
	if perr != nil {
		out.Error = fmt.Sprintf("parse response: %v", perr)
		out.RawBodySnippet = trimSnippet(string(body), 2000)
		return out, nil
	}
	out.Success = true
	out.Assistant = assistant
	return out, nil
}

func accountLabel(a *domain.ModelAccount) string {
	if a == nil {
		return ""
	}
	if a.Name != "" {
		return a.Name + " · " + a.Endpoint
	}
	return a.Endpoint
}

func trimSnippet(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func parseAssistantContent(body []byte) (assistant string, err error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return "", err
	}
	if errRaw, ok := root["error"]; ok {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errRaw, &apiErr)
		if apiErr.Message != "" {
			return "", fmt.Errorf("model account error: %s", apiErr.Message)
		}
		return "", fmt.Errorf("model account returned error field")
	}
	var wrapped struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return "", err
	}
	if len(wrapped.Choices) == 0 {
		return "", fmt.Errorf("empty choices in response")
	}
	c := wrapped.Choices[0].Message.Content
	if c == "" {
		c = wrapped.Choices[0].Delta.Content
	}
	return c, nil
}
