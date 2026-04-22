package remotediscovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trailyai/traffic-ai/internal/domain/provider"
)

const maxBody = 8 << 20 // 8MiB

// RemoteModel 远端返回的单条模型（用于管理台展示与多选）。
type RemoteModel struct {
	ID string `json:"id"`
}

// Result 统一 discover 结果：失败时 Models 为空、Err 非空；调用方可转为 fetch_failed 供前端走手填。
type Result struct {
	Models []RemoteModel
	Err    error
}

// Discover 按商家 Kind 请求远端模型列表。
func Discover(ctx context.Context, def provider.Definition, authType, credential, baseURL string) Result {
	if def.Kind == provider.KindManualOnly {
		return Result{Err: fmt.Errorf("该商家不支持远程拉取列表，请手动填写模型名")}
	}
	base := strings.TrimSpace(baseURL)
	if def.RequireBaseURL && base == "" {
		return Result{Err: fmt.Errorf("请填写 Base URL")}
	}
	if base == "" {
		base = def.DefaultBaseURL
	}
	base = strings.TrimRight(base, "/")

	switch def.Kind {
	case provider.KindOpenAIv1Models:
		return fetchOpenAICompatibleModels(ctx, base, authType, credential)
	case provider.KindAnthropicv1Models:
		return fetchAnthropicModels(ctx, base, credential)
	case provider.KindGeminiList:
		return fetchGeminiModels(ctx, base, credential)
	default:
		return Result{Err: fmt.Errorf("不支持的商家类型")}
	}
}

func fetchOpenAICompatibleModels(ctx context.Context, base, authType, credential string) Result {
	u := base + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Result{Err: err}
	}
	setOpenAIAuth(req, authType, credential)

	resp, err := do(req)
	if err != nil {
		return Result{Err: err}
	}
	defer resp.Body.Close()
	b, err := readBody(resp)
	if err != nil {
		return Result{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{Err: fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(b, 500))}
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return Result{Err: fmt.Errorf("解析模型列表失败: %w", err)}
	}
	models := make([]RemoteModel, 0, len(out.Data))
	for _, d := range out.Data {
		if d.ID != "" {
			models = append(models, RemoteModel{ID: d.ID})
		}
	}
	return Result{Models: models}
}

func fetchAnthropicModels(ctx context.Context, base, credential string) Result {
	u := base + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Result{Err: err}
	}
	req.Header.Set("x-api-key", strings.TrimSpace(credential))
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := do(req)
	if err != nil {
		return Result{Err: err}
	}
	defer resp.Body.Close()
	b, err := readBody(resp)
	if err != nil {
		return Result{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{Err: fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(b, 500))}
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return Result{Err: fmt.Errorf("解析模型列表失败: %w", err)}
	}
	models := make([]RemoteModel, 0, len(out.Data))
	for _, d := range out.Data {
		if d.ID != "" {
			models = append(models, RemoteModel{ID: d.ID})
		}
	}
	return Result{Models: models}
}

func fetchGeminiModels(ctx context.Context, base, credential string) Result {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	uu, err := url.Parse(base + "/models")
	if err != nil {
		return Result{Err: fmt.Errorf("无效的 Base URL")}
	}
	q := uu.Query()
	q.Set("key", strings.TrimSpace(credential))
	uu.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uu.String(), nil)
	if err != nil {
		return Result{Err: err}
	}

	resp, err := do(req)
	if err != nil {
		return Result{Err: err}
	}
	defer resp.Body.Close()
	b, err := readBody(resp)
	if err != nil {
		return Result{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{Err: fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(b, 500))}
	}
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return Result{Err: fmt.Errorf("解析模型列表失败: %w", err)}
	}
	models := make([]RemoteModel, 0, len(out.Models))
	for _, m := range out.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" {
			id = m.Name
		}
		if id != "" {
			models = append(models, RemoteModel{ID: id})
		}
	}
	return Result{Models: models}
}

func setOpenAIAuth(req *http.Request, authType, credential string) {
	cred := strings.TrimSpace(credential)
	switch authType {
	case provider.AuthOAuthBearer, provider.AuthAPIKey:
		req.Header.Set("Authorization", "Bearer "+cred)
	default:
		req.Header.Set("Authorization", "Bearer "+cred)
	}
}

func do(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 45 * time.Second}
	return client.Do(req)
}

func readBody(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
