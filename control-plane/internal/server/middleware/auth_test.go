package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanzoai/agents/control-plane/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeValidator returns either the wrapped APIKey or err. Used to
// drive the middleware's bound-org branches without a database.
type fakeValidator struct {
	want string
	key  *auth.APIKey
	err  error
}

func (f *fakeValidator) Validate(_ context.Context, raw string) (*auth.APIKey, error) {
	if f.err != nil {
		return nil, f.err
	}
	if raw != f.want {
		return nil, auth.ErrKeyInvalid
	}
	return f.key, nil
}

// setupBoundRouter wires APIKeyAuth with a Validator-backed AuthConfig
// and exposes an /api/v1/test handler that echoes the resolved org
// for assertions on the "key wins" property.
func setupBoundRouter(v auth.Validator) *gin.Engine {
	router := gin.New()
	router.Use(APIKeyAuth(AuthConfig{Validator: v}))
	router.GET("/api/v1/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"org":          c.GetString(ContextKeyOrgID),
			"auth_method":  c.GetString(ContextKeyAuthMethod),
			"request_org":  auth.OrgID(c.Request.Context()),
		})
	})
	return router
}

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(config AuthConfig) *gin.Engine {
	router := gin.New()
	router.Use(APIKeyAuth(config))
	router.GET("/api/v1/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})
	router.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	router.GET("/metrics", func(c *gin.Context) {
		c.String(http.StatusOK, "metrics_data")
	})
	router.GET("/ui/index.html", func(c *gin.Context) {
		c.String(http.StatusOK, "<html>UI</html>")
	})
	router.GET("/custom/skip", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "skipped"})
	})
	return router
}

func TestAPIKeyAuth_NoAuthConfigured(t *testing.T) {
	// When no API key is configured, all requests should be allowed
	router := setupRouter(AuthConfig{APIKey: ""})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "success", resp["message"])
}

func TestAPIKeyAuth_ValidXAPIKeyHeader(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "secret-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_ValidBearerToken(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_ValidQueryParam(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test?api_key=secret-key", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	tests := []struct {
		name        string
		headerKey   string
		headerValue string
		queryParam  string
	}{
		{
			name:        "wrong X-API-Key",
			headerKey:   "X-API-Key",
			headerValue: "wrong-key",
		},
		{
			name:        "wrong bearer token",
			headerKey:   "Authorization",
			headerValue: "Bearer wrong-key",
		},
		{
			name:       "wrong query param",
			queryParam: "wrong-key",
		},
		{
			name: "no auth at all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/test"
			if tt.queryParam != "" {
				url += "?api_key=" + tt.queryParam
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerValue)
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)

			var resp map[string]string
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, "unauthorized", resp["error"])
			assert.Contains(t, resp["message"], "invalid or missing API key")
		})
	}
}

func TestAPIKeyAuth_SkipHealthEndpoint(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	// Health endpoint should be accessible without auth
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_SkipHealthSubpaths(t *testing.T) {
	router := gin.New()
	router.Use(APIKeyAuth(AuthConfig{APIKey: "secret-key"}))
	router.GET("/api/v1/health/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_SkipMetricsEndpoint(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_SkipRootHealthEndpoint(t *testing.T) {
	// Root /health endpoint should be accessible without auth for load balancers
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", resp["status"])
}

func TestAPIKeyAuth_SkipUIPath(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/ui/index.html", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_CustomSkipPaths(t *testing.T) {
	router := setupRouter(AuthConfig{
		APIKey:    "secret-key",
		SkipPaths: []string{"/custom/skip"},
	})

	// Custom skip path should be accessible without auth
	req := httptest.NewRequest(http.MethodGet, "/custom/skip", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_XAPIKeyTakesPrecedence(t *testing.T) {
	// If X-API-Key is set, it should be checked first
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	// Valid X-API-Key should succeed even with invalid bearer
	req.Header.Set("X-API-Key", "secret-key")
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_BearerFallback(t *testing.T) {
	// If X-API-Key is empty, should fall back to Bearer token
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "") // Empty, not missing
	req.Header.Set("Authorization", "Bearer secret-key")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyAuth_InvalidBearerFormat(t *testing.T) {
	router := setupRouter(AuthConfig{APIKey: "secret-key"})

	tests := []struct {
		name   string
		header string
	}{
		{"no Bearer prefix", "secret-key"},
		{"Basic auth instead", "Basic secret-key"},
		{"malformed Bearer", "Bearersecret-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestAPIKeyAuth_MultipleSkipPaths(t *testing.T) {
	router := gin.New()
	router.Use(APIKeyAuth(AuthConfig{
		APIKey:    "secret-key",
		SkipPaths: []string{"/public/a", "/public/b", "/public/c"},
	}))
	router.GET("/public/a", func(c *gin.Context) { c.String(http.StatusOK, "a") })
	router.GET("/public/b", func(c *gin.Context) { c.String(http.StatusOK, "b") })
	router.GET("/public/c", func(c *gin.Context) { c.String(http.StatusOK, "c") })
	router.GET("/private", func(c *gin.Context) { c.String(http.StatusOK, "private") })

	// All public paths should be accessible
	for _, path := range []string{"/public/a", "/public/b", "/public/c"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "path %s should be accessible", path)
	}

	// Private path should require auth
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---- Bound-org tests (Red 2026-04-27 P0-2) -------------------------

func TestAPIKeyAuth_BoundKey_MatchingOrg(t *testing.T) {
	v := &fakeValidator{
		want: "hk-good",
		key:  &auth.APIKey{ID: "k1", OrgID: "hanzo", UserID: "u1"},
	}
	router := setupBoundRouter(v)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-good")
	req.Header.Set(auth.HeaderOrgID, "hanzo")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "hanzo", resp["org"])
	assert.Equal(t, "hanzo", resp["request_org"])
	assert.Equal(t, "api_key", resp["auth_method"])
}

func TestAPIKeyAuth_BoundKey_MismatchedOrg_403(t *testing.T) {
	v := &fakeValidator{
		want: "hk-good",
		key:  &auth.APIKey{ID: "k1", OrgID: "hanzo", UserID: "u1"},
	}
	router := setupBoundRouter(v)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-good")
	req.Header.Set(auth.HeaderOrgID, "victim")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "forbidden", resp["error"])
	assert.Contains(t, resp["message"], "different org")
}

func TestAPIKeyAuth_BoundKey_NoOrgHeader_KeyWins(t *testing.T) {
	v := &fakeValidator{
		want: "hk-good",
		key:  &auth.APIKey{ID: "k1", OrgID: "hanzo", UserID: "u1"},
	}
	router := setupBoundRouter(v)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-good")
	// No X-Org-Id supplied — key's bound org is authoritative.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "hanzo", resp["org"])
	assert.Equal(t, "hanzo", resp["request_org"])
}

func TestAPIKeyAuth_BoundKey_InvalidHash_401(t *testing.T) {
	v := &fakeValidator{
		want: "hk-good",
		err:  auth.ErrKeyInvalid,
	}
	router := setupBoundRouter(v)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-bad")
	req.Header.Set(auth.HeaderOrgID, "hanzo")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_BoundKey_Revoked_401(t *testing.T) {
	v := &fakeValidator{want: "hk-good", err: auth.ErrKeyRevoked}
	router := setupBoundRouter(v)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-good")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_BoundKey_Expired_401(t *testing.T) {
	v := &fakeValidator{want: "hk-good", err: auth.ErrKeyExpired}
	router := setupBoundRouter(v)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-good")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKeyAuth_StaticKey_DefersOrgToGateway(t *testing.T) {
	// Static (legacy) key has no org binding. Middleware leaves
	// gin context org untouched so the gateway-trust identity
	// middleware retains authority. Verifies the legacy single-key
	// path keeps working unchanged after the bound-org refactor.
	router := setupBoundRouter(auth.NewStaticValidator("hk-static"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-static")
	req.Header.Set(auth.HeaderOrgID, "hanzo")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Static path: org context is empty here because the test router
	// does not run RequireIdentity. Production routes both — the
	// gateway-trust path pins org from the IAM JWT.
	assert.Empty(t, resp["org"])
	assert.Equal(t, "api_key", resp["auth_method"])
}

func TestAPIKeyAuth_BoundKey_OrgMismatchSentinel(t *testing.T) {
	// Sanity: ensure ErrOrgMismatch is the only path that yields 403.
	// Any other auth.Err* should still be 401.
	v := &fakeValidator{want: "hk-good", err: errors.New("nonsentinel")}
	router := setupBoundRouter(v)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("X-API-Key", "hk-good")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
