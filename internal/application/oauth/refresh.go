package oauth

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
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
)

// RefreshAccessToken 用 refresh_token 向 IdP 的 token_endpoint 获取新的 access_token。
// newRefreshToken 为空表示 IdP 未轮换 refresh_token，调用方应保留原值。
func RefreshAccessToken(ctx context.Context, oauthCfg config.OAuthConfig, providerID, refreshToken string) (
	newAccessToken string, newRefreshToken string, expiresIn int, err error,
) {
	def, ok := provider.ByID(providerID)
	if !ok {
		return "", "", 0, fmt.Errorf("unknown provider: %s", providerID)
	}
	if def.OAuthConfigKey == "" {
		return "", "", 0, fmt.Errorf("provider %s has no OAuth config key", providerID)
	}

	provCfg, ok := oauthCfg.Providers[def.OAuthConfigKey]
	if !ok {
		return "", "", 0, fmt.Errorf("oauth provider config not found for key %s", def.OAuthConfigKey)
	}

	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", refreshToken)
	body.Set("client_id", provCfg.ClientID)
	body.Set("client_secret", provCfg.ClientSecret)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, provCfg.TokenEndpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return "", "", 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", "", 0, fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", "", 0, fmt.Errorf("empty access_token in refresh response")
	}

	return tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn, nil
}
