// Copyright © 2026 Hanzo AI. MIT License.

// Package middleware — HTTP API key auth.
//
// The API key is the trust boundary on the key-auth path: every key
// (issued via pkg/auth.Store) carries its bound OrgID. APIKeyAuth
// loads the key, verifies the request's X-Org-Id matches that bound
// org, and rejects 403 on mismatch. Identity headers from the
// gateway are informational on this path — the key wins.
//
// Closes Red 2026-04-27 P0-2: a valid X-API-Key combined with an
// attacker-chosen X-Org-Id no longer yields cross-tenant access.
package middleware

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hanzoai/agents/control-plane/pkg/auth"
)

// AuthConfig wires the validator and skip-list into the middleware.
// Validator is the canonical shape; APIKey is the legacy single-key
// shortcut that turns into a StaticValidator at config-load time.
type AuthConfig struct {
	Validator auth.Validator
	APIKey    string
	SkipPaths []string
}

// APIKeyAuth enforces API key authentication on every request that
// is not in the skip-set. When AuthConfig.Validator is nil and
// AuthConfig.APIKey is non-empty, we wrap the latter in a
// StaticValidator. When both are empty, the middleware is a pass-
// through (used in the solo/dev path with no key configured).
//
// On success:
//   - sets c.Set("auth_method", "api_key")
//   - if the key has a bound OrgID (Store-backed validator), sets
//     c.Set("iam_user_org", apikey.OrgID) so handlers and the SQL
//     layer agree on the trust pivot. Static keys leave the org
//     untouched — the gateway-trust identity middleware retains
//     authority over org for that path.
//
// On X-Org-Id mismatch with the key's bound org → 403. This is the
// "key is the trust boundary" property Red required.
func APIKeyAuth(config AuthConfig) gin.HandlerFunc {
	skipPathSet := make(map[string]struct{}, len(config.SkipPaths))
	for _, p := range config.SkipPaths {
		skipPathSet[p] = struct{}{}
	}

	validator := config.Validator
	if validator == nil && config.APIKey != "" {
		validator = auth.NewStaticValidator(config.APIKey)
	}

	return func(c *gin.Context) {
		// No auth configured, allow everything.
		if validator == nil {
			c.Next()
			return
		}

		// Skip explicit paths
		if _, ok := skipPathSet[c.Request.URL.Path]; ok {
			c.Next()
			return
		}

		// Always allow health and metrics by default
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/health") || c.Request.URL.Path == "/health" || c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		// Allow UI static files to load (the React app handles auth prompting)
		// Also allow root "/" which redirects to /ui/
		if strings.HasPrefix(c.Request.URL.Path, "/ui") || c.Request.URL.Path == "/" {
			c.Next()
			return
		}

		raw := extractKey(c)
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid or missing API key",
			})
			return
		}

		key, err := validator.Validate(c.Request.Context(), raw)
		if err != nil {
			// All "key is bad" outcomes (invalid, revoked, expired)
			// surface as 401 — distinguishing them in the response
			// would leak whether a particular key value is known.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid or missing API key",
			})
			return
		}

		// Org binding: if the key carries a bound org, enforce it.
		// Empty key.OrgID means "static legacy key" — we defer to
		// the gateway-trust identity headers.
		if key.OrgID != "" {
			requestOrg := c.GetHeader(auth.HeaderOrgID)
			if err := key.CheckOrg(requestOrg); err != nil {
				if errors.Is(err, auth.ErrOrgMismatch) {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
						"error":   "forbidden",
						"message": "api key bound to a different org than request claims",
					})
					return
				}
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error":   "unauthorized",
					"message": "api key validation failed",
				})
				return
			}
			// Key wins: pin org context to the bound org regardless
			// of what the request header claimed (informational).
			c.Set(ContextKeyOrgID, key.OrgID)
			c.Request = c.Request.WithContext(auth.WithOrgContext(c.Request.Context(), key.OrgID))
		}

		c.Set(ContextKeyAuthMethod, "api_key")
		c.Next()
	}
}

// extractKey reads the raw API key from header, bearer token, or
// query param — the same surfaces APIKeyAuth has accepted since
// before the bound-org refactor.
func extractKey(c *gin.Context) string {
	if k := c.GetHeader("X-API-Key"); k != "" {
		return k
	}
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return c.Query("api_key")
}

// constantTimeStringEqual is retained for the gRPC adapter which
// keeps a one-shot path for the legacy single-key check (no validator
// surface on the gRPC entry yet — see grpc_auth.go).
func constantTimeStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
