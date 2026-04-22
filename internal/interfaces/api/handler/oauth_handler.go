package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	appoauth "github.com/trailyai/traffic-ai/internal/application/oauth"
	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

type OAuthHandler struct {
	oauthUC *appoauth.UseCase
}

func NewOAuthHandler(oauthUC *appoauth.UseCase) *OAuthHandler {
	return &OAuthHandler{oauthUC: oauthUC}
}

// RegisterStart 注册需要认证的端点（super_admin 保护）。
func (h *OAuthHandler) RegisterStart(group *gin.RouterGroup) {
	group.POST("/oauth/start", h.StartAuth)
}

// RegisterCallback 注册无需认证的回调端点（浏览器直接跳转，不带 JWT）。
func (h *OAuthHandler) RegisterCallback(r *gin.Engine) {
	r.GET("/admin/oauth/callback", h.HandleCallback)
}

type startAuthReq struct {
	ProviderID string `json:"provider_id" binding:"required"`
}

func (h *OAuthHandler) StartAuth(c *gin.Context) {
	var req startAuthReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ErrBadRequest)
		return
	}

	authURL, err := h.oauthUC.StartAuth(c.Request.Context(), req.ProviderID)
	if err != nil {
		handleError(c, err)
		return
	}
	response.OK(c, gin.H{"auth_url": authURL})
}

func (h *OAuthHandler) HandleCallback(c *gin.Context) {
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		h.renderErrorHTML(c, "缺少 state 或 code 参数")
		return
	}

	result, err := h.oauthUC.HandleCallback(c.Request.Context(), state, code)
	if err != nil {
		h.renderErrorHTML(c, err.Error())
		return
	}

	h.renderSuccessHTML(c, result)
}

func (h *OAuthHandler) renderSuccessHTML(c *gin.Context, result *appoauth.CallbackResult) {
	resultJSON, _ := json.Marshal(result)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>OAuth Callback</title></head>
<body>
<p>授权成功，正在关闭...</p>
<script>
(function(){
  var data = %s;
  if (window.opener) {
    window.opener.postMessage({
      type: 'oauth_callback_success',
      access_token: data.access_token,
      refresh_token: data.refresh_token || '',
      expires_in: data.expires_in,
      provider_id: data.provider_id
    }, '*');
  }
  window.close();
})();
</script>
</body>
</html>`, string(resultJSON))

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func (h *OAuthHandler) renderErrorHTML(c *gin.Context, errMsg string) {
	safeMsg, _ := json.Marshal(errMsg)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>OAuth Error</title></head>
<body>
<p>授权失败：<span id="msg"></span></p>
<p>请关闭此窗口后重试。</p>
<script>
document.getElementById('msg').textContent = %s;
</script>
</body>
</html>`, string(safeMsg))

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
