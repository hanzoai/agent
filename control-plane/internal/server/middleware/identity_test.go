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

func TestRequireIdentity_RejectsEmptyOrgWithUser(t *testing.T) {
	// Empty X-Org-Id with X-User-Id present must 401 in cloud mode:
	// org is the trust pivot for SQL scoping, user is informational.
	// Mirrors pkg/auth.RequireIdentity contract so the gin and stdlib
	// paths reject identically.
	status, _, _, _, _ := runWithIdentity(t, true,
		map[string]string{HeaderUserID: "u-1"},
		"/v1/agents/echo",
	)
	if status != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", status)
	}
}

func TestRequireIdentity_AcceptsOrgWithoutUser(t *testing.T) {
	// Org-only is allowed: this is the canonical M2M shape where
	// the gateway authenticated by API key and emits X-Org-Id but
	// no user. Org pins the request context for SQL scoping.
	status, org, _, _, ctxOrg := runWithIdentity(t, true,
		map[string]string{HeaderOrgID: "hanzo"},
		"/v1/agents/echo",
	)
	if status != http.StatusOK {
		t.Errorf("status: want 200, got %d", status)
	}
	if org != "hanzo" || ctxOrg != "hanzo" {
		t.Errorf("ctx org: gin=%q request=%q", org, ctxOrg)
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
