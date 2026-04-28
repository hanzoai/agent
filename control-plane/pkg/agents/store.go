// Copyright © 2026 Hanzo AI. MIT License.

package agents

import (
	"context"
)

// orgCtxKey is the context key carrying the per-request org id
// flowing from pkg/auth.RequireIdentity → pkg/agents.Store.WithOrg
// → storage filters. Untyped string would collide with foreign keys;
// the unexported type guarantees isolation.
type orgCtxKey struct{}

// WithOrgContext returns ctx with org pinned. Called by the HTTP
// router after pkg/auth populates the request context with the
// gateway-supplied org id.
func WithOrgContext(ctx context.Context, orgID string) context.Context {
	if orgID == "" {
		return ctx
	}
	return context.WithValue(ctx, orgCtxKey{}, orgID)
}

// OrgFromContext returns the active org id for ctx, or "" when none.
// "" means solo mode — queries are unscoped. Production traffic that
// reaches the binary through hanzoai/gateway will always have an org;
// developer/embed traffic without a gateway will not.
func OrgFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(orgCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// Storage is the minimal interface pkg/agents.Store wraps. Kept
// narrow on purpose: the goal of this package is to centralise the
// org-scoping pattern, not to re-export every storage method. The
// existing internal/storage.StorageProvider satisfies this shape
// because every method already takes ctx; the SQL layer reads
// OrgFromContext when adding `WHERE org_id = $1`.
//
// We deliberately do NOT import internal/storage here so the
// fused-binary contract stays orthogonal to any one service's
// storage tree. internal/server passes its provider into NewStore.
type Storage interface {
	// HealthCheck is the only universally-shared method we surface
	// directly. Everything else flows through ctx-injection.
	HealthCheck(ctx context.Context) error
}

// Store is the per-org view onto an underlying Storage. Thin by
// design: storage methods already accept context, so org-scoping
// flows through ctx without a fanout of new method signatures.
//
// New methods that touch tenant data should read OrgFromContext at
// the SQL boundary and add `WHERE org_id = $1`. The migration
// `019_add_org_id.sql` adds the column and indexes.
type Store struct {
	storage Storage
}

// NewStore wraps an underlying storage provider in the org-scoped
// view. The provider must implement the narrow Storage interface;
// internal/storage.StorageProvider does so already.
func NewStore(s Storage) *Store {
	return &Store{storage: s}
}

// WithOrg returns a child Store that pins ctx-org to the supplied
// org id when methods read OrgFromContext.
//
// In production the org flows from the gateway-supplied X-Org-Id
// header through pkg/auth into request ctx; WithOrg is mostly used
// by background jobs and tests where ctx has no org yet.
func (s *Store) WithOrg(orgID string) *OrgView {
	if s == nil {
		return &OrgView{orgID: orgID}
	}
	return &OrgView{store: s, orgID: orgID}
}

// Storage returns the underlying provider for callers that haven't
// migrated to the OrgView API yet. Disappears when every call site
// is org-scoped.
func (s *Store) Storage() Storage {
	if s == nil {
		return nil
	}
	return s.storage
}

// OrgView is a Store pinned to a single org. Methods take a context
// and inject the org via WithOrgContext before calling through to
// the underlying provider.
type OrgView struct {
	store *Store
	orgID string
}

// OrgID returns the pinned org id. "" means unscoped (solo mode).
func (v *OrgView) OrgID() string {
	if v == nil {
		return ""
	}
	return v.orgID
}

// Context returns ctx with this view's org id pinned. The SQL
// builders consult OrgFromContext to add `org_id = $1` predicates.
func (v *OrgView) Context(ctx context.Context) context.Context {
	if v == nil {
		return ctx
	}
	return WithOrgContext(ctx, v.orgID)
}

// Storage returns the underlying provider. Callers should pass
// ctx through OrgView.Context first so SQL builders see the org.
func (v *OrgView) Storage() Storage {
	if v == nil || v.store == nil {
		return nil
	}
	return v.store.storage
}
