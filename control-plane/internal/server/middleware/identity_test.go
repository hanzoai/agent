// Copyright © 2026 Hanzo AI. MIT License.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanzoai/agents/control-plane/pkg/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func runWithIdentity(t *testing.T, require bool, headers map[string]string, path string) (status int, ctxOrg, ctxUser, ctxEmail string, ctxRequestOrg string) {
	t.Helper()
	r := gin.New()
	r.Use(RequireIdentity(require))
	r.GET("/v1/agents/echo", func(c *gin.Context) {
		ctxOrg = c.GetString(ContextKeyOrgID)
		ctxUser = c.GetString(ContextKeyUserID)
		ctxEmail = c.GetString(ContextKeyUserEmail)
		ctxRequestOrg = auth.OrgID(c.Request.Context())
		c.Status(http.StatusOK)
	})
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/_/agents/index.html", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr.Code, ctxOrg, ctxUser, ctxEmail, ctxRequestOrg
}

func TestRequireIdentity_PassesHeadersThrough(t *testing.T) {
	status, org, user, email, ctxOrg := runWithIdentity(t, true,
		map[string]string{
			HeaderOrgID:     "hanzo",
			HeaderUserID:    "u-123",
			HeaderUserEmail: "z@hanzo.ai",
		},
		"/v1/agents/echo",
	)

	if status != http.StatusOK {
		t.Errorf("status: want 200, got %d", status)
	}
	if org != "hanzo" || user != "u-123" || email != "z@hanzo.ai" {
		t.Errorf("gin ctx: org=%q user=%q email=%q", org, user, email)
	}
	if ctxOrg != "hanzo" {
		t.Errorf("request ctx OrgID: want hanzo, got %q", ctxOrg)
	}
}

func TestRequireIdentity_RejectsMissingInCloud(t *testing.T) {
	status, _, _, _, _ := runWithIdentity(t, true, nil, "/v1/agents/echo")
	if status != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", status)
	}
}

func TestRequireIdentity_AllowsMissingInSolo(t *testing.T) {
	status, _, _, _, _ := runWithIdentity(t, false, nil, "/v1/agents/echo")
	if status != http.StatusOK {
		t.Errorf("status: want 200, got %d", status)
	}
}

func TestRequireIdentity_PublicPathsAlwaysAllowed(t *testing.T) {
	for _, path := range []string{"/health", "/_/agents/index.html"} {
		status, _, _, _, _ := runWithIdentity(t, true, nil, path)
		if status != http.StatusOK {
			t.Errorf("path %s: want 200 in cloud mode without identity, got %d", path, status)
		}
	}
}
