package model

import (
	"bytes"
	"context"
	"encoding/base64"
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

// 上游 Playground 响应体大小上限。图片接口 JSON 内嵌完整 base64，曾用 64KB 截断会导致
// 「unexpected end of JSON input」且管理端无法展示缩略图。
const (
	maxPlaygroundGenericBody int64 = 4 << 20 // 4MB：chat / embeddings
	maxPlaygroundImageBody   int64 = 32 << 20 // 32MB：images/generations（含 b64_json）
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
	// ResultKind：chat | embedding | image，便于前端切换展示。
	ResultKind string `json:"result_kind,omitempty"`
	// ImageDataURL 仅 image 成功时：data:image/png;base64,... 便于控制台内嵌预览。
	ImageDataURL string `json:"image_data_url,omitempty"`
}

// PlaygroundChat 向模型对应账号发调试请求：按 model_type 选择 chat / embeddings / images。
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

	switch playgroundEffectiveKind(m) {
	case "speech":
		return &PlaygroundResult{
			Success: false,
			Model:   m.ModelName,
			Account: accountLabel(target),
			Error:   "管理端 Playground 暂不支持语音（TTS）模型，请使用用户控制台的「对话测试」并选择「OpenAI 语音」接口。",
		}, nil
	case "embedding":
		return uc.playgroundEmbeddings(ctx, m, target, plain, messages)
	case "image":
		return uc.playgroundImage(ctx, m, target, plain, messages)
	default:
		return uc.playgroundChatOpenAI(ctx, m, target, plain, messages, maxTokens)
	}
}

// playgroundEffectiveKind 与连通性探测一致的启发式：未标类型时按模型名推断。
func playgroundEffectiveKind(m *domain.Model) string {
	mt := strings.ToLower(strings.TrimSpace(m.ModelType))
	switch mt {
	case "embedding", "image", "speech":
		return mt
	}
	ln := strings.ToLower(m.ModelName)
	if strings.HasPrefix(ln, "gpt-image") || strings.Contains(ln, "dall-e") {
		return "image"
	}
	if strings.Contains(ln, "embed") || strings.HasPrefix(ln, "text-embedding") {
		return "embedding"
	}
	return "chat"
}

func firstUserContent(messages []map[string]string) string {
	for _, msg := range messages {
		if strings.EqualFold(msg["role"], "user") && strings.TrimSpace(msg["content"]) != "" {
			return strings.TrimSpace(msg["content"])
		}
	}
	if len(messages) > 0 && strings.TrimSpace(messages[0]["content"]) != "" {
		return strings.TrimSpace(messages[0]["content"])
	}
	return ""
}

func (uc *UseCase) playgroundChatOpenAI(ctx context.Context, m *domain.Model, target *domain.ModelAccount, plain string, messages []map[string]string, maxTokens int) (*PlaygroundResult, error) {
	if len(messages) == 0 {
		messages = []map[string]string{{"role": "user", "content": "Say hello in one short sentence."}}
	}
	if maxTokens <= 0 {
		maxTokens = 256
	}
	if maxTokens > 4096 {
		maxTokens = 4096
	}

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
	return uc.playgroundDoJSON(ctx, m, target, plain, url, reqBody, "chat", 60*time.Second, parseAssistantContent)
}

func (uc *UseCase) playgroundEmbeddings(ctx context.Context, m *domain.Model, target *domain.ModelAccount, plain string, messages []map[string]string) (*PlaygroundResult, error) {
	input := firstUserContent(messages)
	if input == "" {
		input = "traffic playground embedding probe"
	}
	payload := map[string]interface{}{
		"model": m.ModelName,
		"input": input,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return &PlaygroundResult{Success: false, Model: m.ModelName, Account: accountLabel(target), ResultKind: "embedding", Error: fmt.Sprintf("build json: %v", err)}, nil
	}
	url := upstreamurl.JoinPath(target.Endpoint, "/embeddings")
	return uc.playgroundDoJSON(ctx, m, target, plain, url, reqBody, "embedding", 60*time.Second, parseEmbeddingPreview)
}

func (uc *UseCase) playgroundImage(ctx context.Context, m *domain.Model, target *domain.ModelAccount, plain string, messages []map[string]string) (*PlaygroundResult, error) {
	prompt := firstUserContent(messages)
	if prompt == "" {
		prompt = "A minimal abstract texture, soft gradient, single color theme."
	}
	payload := map[string]interface{}{
		"prompt":  prompt,
		"n":       1,
		"size":    "1024x1024",
		"quality": "low",
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return &PlaygroundResult{Success: false, Model: m.ModelName, Account: accountLabel(target), ResultKind: "image", Error: fmt.Sprintf("build json: %v", err)}, nil
	}
	url := upstreamurl.JoinPath(target.Endpoint, "/images/generations")
	return uc.playgroundDoImage(ctx, m, target, plain, url, reqBody)
}

type bodyParser func(body []byte) (assistant string, imageDataURL string, err error)

func (uc *UseCase) playgroundDoJSON(ctx context.Context, m *domain.Model, target *domain.ModelAccount, plain, url string, reqBody []byte, kind string, timeout time.Duration, parse bodyParser) (*PlaygroundResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return &PlaygroundResult{Success: false, Model: m.ModelName, Account: accountLabel(target), ResultKind: kind, Error: fmt.Sprintf("build request: %v", err)}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+plain)

	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Do(httpReq)
	latency := time.Since(start)
	out := &PlaygroundResult{
		Model:      m.ModelName,
		Account:    accountLabel(target),
		LatencyMs:  int(latency.Milliseconds()),
		ResultKind: kind,
	}
	if err != nil {
		out.Error = fmt.Sprintf("request failed: %v", err)
		return out, nil
	}
	defer resp.Body.Close()
	body, errRead := io.ReadAll(http.MaxBytesReader(nil, resp.Body, maxPlaygroundGenericBody))
	if errRead != nil {
		out.Error = fmt.Sprintf("read body: %v", errRead)
		return out, nil
	}
	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		out.RawBodySnippet = trimSnippet(string(body), 2000)
		return out, nil
	}
	text, imgURL, perr := parse(body)
	if perr != nil {
		out.Error = fmt.Sprintf("parse response: %v", perr)
		out.RawBodySnippet = trimSnippet(string(body), 2000)
		return out, nil
	}
	out.Success = true
	out.Assistant = text
	out.ImageDataURL = imgURL
	return out, nil
}

func (uc *UseCase) playgroundDoImage(ctx context.Context, m *domain.Model, target *domain.ModelAccount, plain, url string, reqBody []byte) (*PlaygroundResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return &PlaygroundResult{Success: false, Model: m.ModelName, Account: accountLabel(target), ResultKind: "image", Error: fmt.Sprintf("build request: %v", err)}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+plain)

	client := &http.Client{Timeout: 120 * time.Second}
	start := time.Now()
	resp, err := client.Do(httpReq)
	latency := time.Since(start)
	out := &PlaygroundResult{
		Model:      m.ModelName,
		Account:    accountLabel(target),
		LatencyMs:  int(latency.Milliseconds()),
		ResultKind: "image",
	}
	if err != nil {
		out.Error = fmt.Sprintf("request failed: %v", err)
		return out, nil
	}
	defer resp.Body.Close()
	body, errRead := io.ReadAll(http.MaxBytesReader(nil, resp.Body, maxPlaygroundImageBody))
	if errRead != nil {
		out.Error = fmt.Sprintf("read body: %v", errRead)
		return out, nil
	}
	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		out.RawBodySnippet = trimSnippet(string(body), 2000)
		return out, nil
	}
	text, imgURL, perr := parseImageResponse(body)
	if perr != nil {
		out.Error = fmt.Sprintf("parse response: %v", perr)
		out.RawBodySnippet = trimSnippet(string(body), 2000)
		return out, nil
	}
	out.Success = true
	out.Assistant = text
	out.ImageDataURL = imgURL
	return out, nil
}

func parseEmbeddingPreview(body []byte) (string, string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return "", "", err
	}
	if errRaw, ok := root["error"]; ok {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errRaw, &apiErr)
		if apiErr.Message != "" {
			return "", "", fmt.Errorf("%s", apiErr.Message)
		}
		return "", "", fmt.Errorf("upstream error field")
	}
	var wrapped struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return "", "", err
	}
	if len(wrapped.Data) == 0 || len(wrapped.Data[0].Embedding) == 0 {
		return "", "", fmt.Errorf("empty embedding data")
	}
	emb := wrapped.Data[0].Embedding
	dim := len(emb)
	previewN := 8
	if previewN > dim {
		previewN = dim
	}
	parts := make([]string, previewN)
	for i := 0; i < previewN; i++ {
		parts[i] = fmt.Sprintf("%.6f", emb[i])
	}
	summary := fmt.Sprintf("向量维度 %d，前 %d 维预览: [%s]", dim, previewN, strings.Join(parts, ", "))
	if dim > previewN {
		summary += " …"
	}
	return summary, "", nil
}

func parseImageResponse(body []byte) (string, string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return "", "", err
	}
	if errRaw, ok := root["error"]; ok {
		var apiErr struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errRaw, &apiErr)
		if apiErr.Message != "" {
			return "", "", fmt.Errorf("%s", apiErr.Message)
		}
		return "", "", fmt.Errorf("upstream error field")
	}
	var wrapped struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return "", "", err
	}
	if len(wrapped.Data) == 0 {
		return "", "", fmt.Errorf("empty data array")
	}
	d := wrapped.Data[0]
	if d.URL != "" {
		return fmt.Sprintf("图片 URL（请浏览器打开）: %s", d.URL), "", nil
	}
	if d.B64JSON == "" {
		return "", "", fmt.Errorf("no b64_json or url in response")
	}
	mime := http.DetectContentType(mustDecodeHead(d.B64JSON))
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	dataURL := fmt.Sprintf("data:%s;base64,%s", mime, d.B64JSON)
	summary := fmt.Sprintf("已生成图片（base64 长度 %d）", len(d.B64JSON))
	return summary, dataURL, nil
}

func mustDecodeHead(b64 string) []byte {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) < 12 {
		return []byte{0x89, 0x50, 0x4E, 0x47}
	}
	return raw[:min(512, len(raw))]
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

func parseAssistantContent(body []byte) (assistant string, _ string, err error) {
	a, err := parseAssistantContentChat(body)
	return a, "", err
}

func parseAssistantContentChat(body []byte) (string, error) {
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
