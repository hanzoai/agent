// Copyright © 2026 Hanzo AI. MIT License.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestStaticValidator_NoConfiguredKey(t *testing.T) {
	v := NewStaticValidator("")
	if _, err := v.Validate(context.Background(), "anything"); !errors.Is(err, ErrKeyInvalid) {
		t.Errorf("err: want ErrKeyInvalid, got %v", err)
	}
}

func TestStaticValidator_Match(t *testing.T) {
	v := NewStaticValidator("hk-static-key-1234567890abcdef")
	k, err := v.Validate(context.Background(), "hk-static-key-1234567890abcdef")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if k.OrgID != "" {
		t.Errorf("static OrgID: want empty (defers to gateway), got %q", k.OrgID)
	}
	if k.ID != "static" {
		t.Errorf("ID: want static, got %q", k.ID)
	}
}

func TestStaticValidator_Mismatch(t *testing.T) {
	v := NewStaticValidator("hk-static-key-1234567890abcdef")
	if _, err := v.Validate(context.Background(), "hk-wrong"); !errors.Is(err, ErrKeyInvalid) {
		t.Errorf("err: want ErrKeyInvalid, got %v", err)
	}
}

func TestAPIKey_CheckOrg(t *testing.T) {
	k := &APIKey{OrgID: "hanzo"}

	if err := k.CheckOrg(""); err != nil {
		t.Errorf("empty request org: want nil, got %v", err)
	}
	if err := k.CheckOrg("hanzo"); err != nil {
		t.Errorf("matching org: want nil, got %v", err)
	}
	if err := k.CheckOrg("attacker"); !errors.Is(err, ErrOrgMismatch) {
		t.Errorf("mismatched org: want ErrOrgMismatch, got %v", err)
	}
}

func TestHashKey_RoundTrip(t *testing.T) {
	raw := "hk-1234567890abcdef1234567890abcdef"
	hash, err := HashKey(raw)
	if err != nil {
		t.Fatalf("HashKey: %v", err)
	}
	if hash == raw || len(hash) < 50 {
		t.Errorf("hash looks unhashed: %q", hash)
	}
}

func TestPrefixOf(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"hk-12345678abcdef", "hk-12345678"},
		{"hk-short", "hk-short"},
		{"", ""},
	}
	for _, tt := range tests {
		got := prefixOf(tt.raw)
		if got != tt.want {
			t.Errorf("prefixOf(%q): want %q, got %q", tt.raw, tt.want, got)
		}
	}
}

func TestSplitScopes(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a,b,c", 3},
		{"a,,b", 2},
	}
	for _, tt := range tests {
		got := splitScopes(tt.in)
		if len(got) != tt.want {
			t.Errorf("splitScopes(%q): want len %d, got %d (%v)", tt.in, tt.want, len(got), got)
		}
	}
}

// TestStore_Validate exercises the Postgres-backed path. Skipped
// unless TEST_POSTGRES_DSN is set so unit runs stay hermetic; the
// integration path is hit by CI's compose-up DB.
func TestStore_Validate(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping integration test")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS api_keys (
		    id TEXT PRIMARY KEY,
		    prefix TEXT NOT NULL,
		    hash TEXT NOT NULL,
		    org_id TEXT NOT NULL CHECK (org_id <> ''),
		    user_id TEXT NOT NULL CHECK (user_id <> ''),
		    scopes TEXT NOT NULL DEFAULT '',
		    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		    expires_at TIMESTAMPTZ,
		    revoked_at TIMESTAMPTZ
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `TRUNCATE api_keys`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	raw := "hk-test1234abcd1234abcd1234abcd1234"
	hash, err := HashKey(raw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO api_keys (id, prefix, hash, org_id, user_id, scopes)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, "k1", raw[:KeyPrefixLen], hash, "hanzo", "z@hanzo.ai", "read,write"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	store := NewStore(db)

	// Happy path
	k, err := store.Validate(ctx, raw)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if k.OrgID != "hanzo" {
		t.Errorf("OrgID: want hanzo, got %q", k.OrgID)
	}
	if len(k.Scopes) != 2 {
		t.Errorf("scopes: want 2, got %v", k.Scopes)
	}

	// Invalid (different bytes, same prefix → bcrypt mismatch)
	wrong := raw[:KeyPrefixLen] + "WRONGSUFFIXWRONGSUFFIXWRONGSUFFIX"
	if _, err := store.Validate(ctx, wrong); !errors.Is(err, ErrKeyInvalid) {
		t.Errorf("wrong key: want ErrKeyInvalid, got %v", err)
	}

	// Revoked
	if _, err := db.ExecContext(ctx, `UPDATE api_keys SET revoked_at = now() WHERE id = 'k1'`); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := store.Validate(ctx, raw); !errors.Is(err, ErrKeyRevoked) {
		t.Errorf("revoked: want ErrKeyRevoked, got %v", err)
	}

	// Expired
	if _, err := db.ExecContext(ctx, `
		UPDATE api_keys SET revoked_at = NULL, expires_at = $1 WHERE id = 'k1'
	`, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if _, err := store.Validate(ctx, raw); !errors.Is(err, ErrKeyExpired) {
		t.Errorf("expired: want ErrKeyExpired, got %v", err)
	}

	// No row
	if _, err := store.Validate(ctx, "hk-nosuch1234nosuch1234nosuch1234"); !errors.Is(err, ErrKeyInvalid) {
		t.Errorf("no row: want ErrKeyInvalid, got %v", err)
	}
}
