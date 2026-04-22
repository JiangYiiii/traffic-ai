package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trailyai/traffic-ai/internal/domain/provider"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/logger"
)

type UseCase struct {
	oauthCfg  config.OAuthConfig
	stateRepo *mysql.OAuthStateRepo
	aesKey    []byte
}

func NewUseCase(oauthCfg config.OAuthConfig, stateRepo *mysql.OAuthStateRepo, aesKey []byte) *UseCase {
	return &UseCase{oauthCfg: oauthCfg, stateRepo: stateRepo, aesKey: aesKey}
}

// CallbackResult 是 token 交换的返回结构。
type CallbackResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	ProviderID   string `json:"provider_id"`
}

// StartAuth 发起 OAuth 2.0 + PKCE 授权，返回授权 URL。
func (uc *UseCase) StartAuth(ctx context.Context, providerID string) (string, error) {
	def, ok := provider.ByID(providerID)
	if !ok {
		return "", errcode.ErrOAuthNotConfigured
	}
	if def.OAuthConfigKey == "" {
		return "", errcode.ErrOAuthNotConfigured
	}

	provCfg, ok := uc.oauthCfg.Providers[def.OAuthConfigKey]
	if !ok {
		return "", errcode.ErrOAuthNotConfigured
	}

	state, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	codeVerifier, err := randomURLSafe(64)
	if err != nil {
		return "", fmt.Errorf("generate code_verifier: %w", err)
	}

	codeChallenge := s256Challenge(codeVerifier)

	if err := uc.stateRepo.Create(ctx, state, providerID, codeVerifier, nil); err != nil {
		logger.L.Errorw("oauth: create state failed", "err", err, "provider", providerID)
		return "", errcode.ErrInternal
	}

	redirectURI := strings.TrimRight(uc.oauthCfg.PublicBaseURL, "/") + "/admin/oauth/callback"

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", provCfg.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", provCfg.Scopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	authURL := provCfg.AuthorizationEndpoint + "?" + params.Encode()

	logger.L.Infow("oauth: start auth", "provider", providerID, "state", state)
	return authURL, nil
}

// HandleCallback 用 authorization_code + PKCE 换取 token。
func (uc *UseCase) HandleCallback(ctx context.Context, state, code string) (*CallbackResult, error) {
	providerID, codeVerifier, _, err := uc.stateRepo.Consume(ctx, state)
	if err != nil {
		logger.L.Infow("oauth: consume state failed", "err", err, "state", state)
		return nil, errcode.ErrOAuthStateFailed
	}

	def, _ := provider.ByID(providerID)
	provCfg, ok := uc.oauthCfg.Providers[def.OAuthConfigKey]
	if !ok {
		return nil, errcode.ErrOAuthNotConfigured
	}

	redirectURI := strings.TrimRight(uc.oauthCfg.PublicBaseURL, "/") + "/admin/oauth/callback"

	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", redirectURI)
	body.Set("client_id", provCfg.ClientID)
	body.Set("client_secret", provCfg.ClientSecret)
	body.Set("code_verifier", codeVerifier)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.PostForm(provCfg.TokenEndpoint, body)
	if err != nil {
		logger.L.Errorw("oauth: token exchange request failed", "err", err, "provider", providerID)
		return nil, errcode.ErrOAuthTokenExchange
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.L.Errorw("oauth: read token response failed", "err", err, "provider", providerID)
		return nil, errcode.ErrOAuthTokenExchange
	}

	if resp.StatusCode != http.StatusOK {
		logger.L.Errorw("oauth: token endpoint returned error",
			"provider", providerID,
			"status", resp.StatusCode,
			"body", string(respBody),
		)
		return nil, errcode.ErrOAuthTokenExchange
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		logger.L.Errorw("oauth: parse token response failed", "err", err, "body", string(respBody))
		return nil, errcode.ErrOAuthTokenExchange
	}

	if tokenResp.AccessToken == "" {
		logger.L.Errorw("oauth: empty access_token in response", "body", string(respBody))
		return nil, errcode.ErrOAuthTokenExchange
	}

	logger.L.Infow("oauth: token exchange success", "provider", providerID)
	return &CallbackResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		ProviderID:   providerID,
	}, nil
}

// randomHex 生成 n 字节的十六进制字符串（长度 2n）。
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randomURLSafe 生成 n 字符的 URL-safe 随机字符串（用于 PKCE code_verifier）。
func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}

// s256Challenge 计算 PKCE S256 code_challenge = base64url_no_pad(sha256(verifier))。
func s256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
