// Copyright © 2026 Hanzo AI. MIT License.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireIdentity_RequireFalseAllowsMissing(t *testing.T) {
	mw := RequireIdentity(false)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := OrgID(r.Context()); got != "" {
			t.Errorf("OrgID: want empty, got %q", got)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("downstream not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", rr.Code)
	}
}

func TestRequireIdentity_RequireTrueRejectsMissing(t *testing.T) {
	mw := RequireIdentity(true)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if called {
		t.Fatal("downstream called despite missing identity")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rr.Code)
	}
}

func TestRequireIdentity_PropagatesHeaders(t *testing.T) {
	mw := RequireIdentity(true)
	var (
		gotOrg, gotUser, gotEmail string
	)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrg = OrgID(r.Context())
		gotUser = UserID(r.Context())
		gotEmail = UserEmail(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderOrgID, "hanzo")
	req.Header.Set(HeaderUserID, "user-123")
	req.Header.Set(HeaderUserEmail, "z@hanzo.ai")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rr.Code)
	}
	if gotOrg != "hanzo" {
		t.Errorf("OrgID: want hanzo, got %q", gotOrg)
	}
	if gotUser != "user-123" {
		t.Errorf("UserID: want user-123, got %q", gotUser)
	}
	if gotEmail != "z@hanzo.ai" {
		t.Errorf("UserEmail: want z@hanzo.ai, got %q", gotEmail)
	}
}

func TestRequireIdentity_RequireTrueRejectsUserOnly(t *testing.T) {
	// Empty X-Org-Id collapses to the solo bucket and reaches
	// handlers with unscoped queries until 021_org_id_not_null
	// lands. Cloud mode rejects regardless of X-User-Id presence:
	// org is the trust pivot, user is informational.
	mw := RequireIdentity(true)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderUserID, "user-anon")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if called {
		t.Fatal("downstream called despite empty X-Org-Id")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rr.Code)
	}
}

func TestRequireIdentity_RequireTrueAcceptsOrgOnly(t *testing.T) {
	// Org-only is the canonical M2M shape: gateway has authenticated
	// the caller against its API key (or another path) and emits
	// X-Org-Id without a user. Allowed because org pins SQL scoping.
	mw := RequireIdentity(true)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := OrgID(r.Context()); got != "hanzo" {
			t.Errorf("OrgID: want hanzo, got %q", got)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderOrgID, "hanzo")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("downstream not called despite org header")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", rr.Code)
	}
}

func TestOrgID_EmptyContext(t *testing.T) {
	if got := OrgID(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != "" {
		t.Errorf("OrgID(empty ctx): want empty, got %q", got)
	}
}
