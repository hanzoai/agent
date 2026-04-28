// Copyright © 2026 Hanzo AI. MIT License.

// Package middleware now hosts the gateway-trust gin adapter.
//
// In-binary IAM JWT validation is gone (was iam_auth.go, deleted
// 2026-04-27). hanzoai/gateway is the trust boundary: it validates
// the IAM JWT, strips client-supplied identity headers, and emits
// X-Org-Id, X-User-Id, X-User-Email after successful verification.
// The binary trusts those headers — that's the canonical Hanzo
// binary shape, see ~/work/hanzo/HANZO_BINARY.md.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hanzoai/agents/control-plane/pkg/auth"
)

const (
	// HeaderOrgID, HeaderUserID, HeaderUserEmail are vendor-free
	// X-* headers populated by hanzoai/gateway after IAM JWT
	// validation (per ~/work/hanzo/CLAUDE.md HTTP-header convention
	// 2026-03-27).
	HeaderOrgID     = auth.HeaderOrgID
	HeaderUserID    = auth.HeaderUserID
	HeaderUserEmail = auth.HeaderUserEmail

	// Gin context keys for the gateway-supplied identity. Kept
	// stable so existing handlers that read c.GetString(...) keep
	// working through the migration.
	ContextKeyOrgID      = "iam_user_org"
	ContextKeyUserID     = "iam_user_id"
	ContextKeyUserEmail  = "iam_user_email"
	ContextKeyAuthMethod = "auth_method"
)

// RequireIdentity is the canonical gin middleware: it reads
// gateway-supplied identity headers, attaches them to ctx (so
// pkg/agents.OrgFromContext works) and to the gin context (so
// existing handlers compile unchanged).
//
// require=true → 401 when no identity headers are present (cloud).
// require=false → solo path, headers are optional.
//
// Public probes and the SPA shell are mounted outside this
// middleware via skipPublicPaths, mirroring the route shape
// ~/work/hanzo/tasks/cmd/tasksd/main.go uses.
func RequireIdentity(require bool) gin.HandlerFunc {
	skip := skipPublicPaths()
	return func(c *gin.Context) {
		if skip(c.Request.URL.Path) {
			c.Next()
			return
		}

		org := c.GetHeader(HeaderOrgID)
		user := c.GetHeader(HeaderUserID)
		email := c.GetHeader(HeaderUserEmail)

		// Empty X-Org-Id collapses to the solo bucket and falls
		// through to handlers with empty org → unscoped queries
		// until 021_org_id_not_null lands. Defense-in-depth: in
		// cloud mode we reject any request without an org header
		// regardless of X-User-Id presence. Mirrors pkg/auth
		// middleware so the gin and stdlib paths are byte-for-byte
		// equivalent.
		if require && org == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "identity required",
				"code":  401,
			})
			return
		}

		// Stamp the gin context for handlers reading c.GetString(...).
		c.Set(ContextKeyOrgID, org)
		c.Set(ContextKeyUserID, user)
		c.Set(ContextKeyUserEmail, email)
		if user != "" {
			c.Set(ContextKeyAuthMethod, "iam")
		}

		// Stamp the request context so pkg/agents.OrgFromContext
		// resolves at the SQL layer.
		c.Request = c.Request.WithContext(auth.WithOrgContext(c.Request.Context(), org))

		c.Next()
	}
}

// skipPublicPaths returns a predicate that keeps probes and the
// SPA shell unauthenticated. Same path set the deleted CombinedAuth
// used so the new shape is a strict subset.
func skipPublicPaths() func(path string) bool {
	prefixes := []string{
		"/health",
		"/healthz",
		"/metrics",
		"/v1/health",
		"/v1/agents/health",
		"/ui",
		"/_/agents/",
	}
	return func(path string) bool {
		if path == "/" {
			return true
		}
		for _, p := range prefixes {
			if strings.HasPrefix(path, p) {
				return true
			}
		}
		return false
	}
}
