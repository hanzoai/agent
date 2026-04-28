// Copyright © 2026 Hanzo AI. MIT License.

package agents

import (
	"context"
	"testing"
)

func TestOrgFromContext_Empty(t *testing.T) {
	if got := OrgFromContext(context.Background()); got != "" {
		t.Errorf("OrgFromContext(empty): want empty, got %q", got)
	}
}

func TestWithOrgContext_RoundTrip(t *testing.T) {
	ctx := WithOrgContext(context.Background(), "hanzo")
	if got := OrgFromContext(ctx); got != "hanzo" {
		t.Errorf("OrgFromContext: want hanzo, got %q", got)
	}
}

func TestWithOrgContext_EmptyDoesNotPin(t *testing.T) {
	// Solo mode: passing "" should not poison ctx with a sentinel.
	ctx := WithOrgContext(context.Background(), "")
	if got := OrgFromContext(ctx); got != "" {
		t.Errorf("OrgFromContext(empty pin): want empty, got %q", got)
	}
}

func TestStore_WithOrg(t *testing.T) {
	st := NewStore(nil)
	view := st.WithOrg("hanzo")
	if view.OrgID() != "hanzo" {
		t.Errorf("OrgID: want hanzo, got %q", view.OrgID())
	}

	ctx := view.Context(context.Background())
	if got := OrgFromContext(ctx); got != "hanzo" {
		t.Errorf("OrgFromContext via OrgView.Context: want hanzo, got %q", got)
	}
}

func TestStore_WithOrg_EmptyView(t *testing.T) {
	// An OrgView with "" represents the solo path. Context propagation
	// must keep it unscoped, not poisoned with an empty sentinel.
	st := NewStore(nil)
	view := st.WithOrg("")
	if view.OrgID() != "" {
		t.Errorf("OrgID: want empty, got %q", view.OrgID())
	}

	ctx := view.Context(context.Background())
	if got := OrgFromContext(ctx); got != "" {
		t.Errorf("OrgFromContext on empty view: want empty, got %q", got)
	}
}

func TestStore_NilSafe(t *testing.T) {
	// Defensive: nil Store should not panic when callers chain.
	var s *Store
	if s.Storage() != nil {
		t.Error("nil Store.Storage(): want nil")
	}
	v := s.WithOrg("hanzo")
	if v.OrgID() != "hanzo" {
		t.Errorf("nil Store.WithOrg: want hanzo, got %q", v.OrgID())
	}
}
