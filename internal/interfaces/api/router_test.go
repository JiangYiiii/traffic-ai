package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestControlRouterStaticNoRouteWithAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	g := r.Group("")
	g.POST("/auth/register/send-code", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	g.GET("/admin/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	staticH := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("static"))
	})
	r.NoRoute(staticNoRouteHandler("", staticH))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.html", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /app.html = %d, want 200", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/register/send-code", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("POST /auth/register/send-code = %d, want 204", w.Code)
	}
}
