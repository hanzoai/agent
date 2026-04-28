// Copyright © 2026 Hanzo AI. MIT License.

// Package auth wires gateway-supplied identity headers (X-Org-Id,
// X-User-Id, X-User-Email) into request context. agentd does not
// validate JWTs itself — hanzoai/gateway already did that. The
// middleware is the trust boundary: in production, only the gateway
// can reach agentd, and AGENTD_REQUIRE_IDENTITY=true rejects any
// request without identity headers.
package auth

import (
	"context"
	"net/http"
)

const (
	HeaderOrgID     = "X-Org-Id"
	HeaderUserID    = "X-User-Id"
	HeaderUserEmail = "X-User-Email"
)

type ctxKey int

const (
	ctxKeyOrgID ctxKey = iota
	ctxKeyUserID
	ctxKeyUserEmail
)

// RequireIdentity reads identity headers and attaches them to ctx.
// When require=true and no headers are present, responds 401. When
// require=false, missing headers yield empty ctx values — the
// embedded/dev path.
func RequireIdentity(require bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			org := r.Header.Get(HeaderOrgID)
			user := r.Header.Get(HeaderUserID)
			email := r.Header.Get(HeaderUserEmail)

			if require && org == "" && user == "" {
				http.Error(w, `{"error":"identity required","code":401}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxKeyOrgID, org)
			ctx = context.WithValue(ctx, ctxKeyUserID, user)
			ctx = context.WithValue(ctx, ctxKeyUserEmail, email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OrgID returns the org id attached by RequireIdentity, or "".
func OrgID(ctx context.Context) string { return strFromCtx(ctx, ctxKeyOrgID) }

// UserID returns the user id attached by RequireIdentity, or "".
func UserID(ctx context.Context) string { return strFromCtx(ctx, ctxKeyUserID) }

// UserEmail returns the user email attached by RequireIdentity, or "".
func UserEmail(ctx context.Context) string { return strFromCtx(ctx, ctxKeyUserEmail) }

// WithOrgContext returns ctx with org pinned. Used by the gin
// adapter (and any non-stdlib transport) so the SQL layer can read
// OrgID(ctx) without going through the gin Request.
func WithOrgContext(ctx context.Context, orgID string) context.Context {
	if orgID == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyOrgID, orgID)
}

func strFromCtx(ctx context.Context, k ctxKey) string {
	if v, ok := ctx.Value(k).(string); ok {
		return v
	}
	return ""
}
