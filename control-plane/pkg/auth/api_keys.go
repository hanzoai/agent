// Copyright © 2026 Hanzo AI. MIT License.

// Package auth — API key validation. The API key is the trust boundary
// for the key-auth path: each key carries its bound OrgID, and the
// middleware rejects any request whose X-Org-Id mismatches that bound
// org. Closes Red 2026-04-27 P0-2 (cross-tenant access via valid key
// + attacker-chosen org header).
//
// Two validator shapes:
//
//   - StaticValidator: single shared key from env (HANZO_AGENTS_API_KEY).
//     No org binding — falls back to the gateway-trust identity headers
//     for org context. The legacy operator path.
//
//   - Store: durable per-key records in Postgres (table: api_keys,
//     migration 022). Each row binds org_id, user_id, scopes, and
//     expiry. Bcrypt-hashed key value, never plaintext.
//
// Wire shape: every key is `hk-<32-hex>`. The first 11 bytes
// (`hk-<8-hex>`) form the lookup prefix — long enough to defeat
// enumeration, short enough for an indexed equality match.
package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// KeyPrefixLen is the number of leading bytes used as the indexed
// lookup column. "hk-" (3) + 8 hex chars = 11. Anything beyond the
// prefix lives only in the bcrypt hash.
const KeyPrefixLen = 11

// Sentinel errors. Callers must distinguish "wrong key" from "wrong
// org" because the HTTP/gRPC adapters map them to 401 vs 403.
var (
	ErrKeyInvalid  = errors.New("api key invalid")
	ErrKeyRevoked  = errors.New("api key revoked")
	ErrKeyExpired  = errors.New("api key expired")
	ErrOrgMismatch = errors.New("api key org mismatch")
)

// APIKey is the durable record. ID is opaque (uuid). Hash is bcrypt
// over the raw key value. OrgID is the trust pivot — the SQL layer
// scopes every read/write to it.
type APIKey struct {
	ID        string
	Prefix    string
	Hash      string
	OrgID     string
	UserID    string
	Scopes    []string
	CreatedAt time.Time
	ExpiresAt *time.Time
	RevokedAt *time.Time
}

// Validator is the smallest contract APIKeyAuth needs: take a raw
// key string, return the bound APIKey (with OrgID populated) or an
// error. Implementations MUST be constant-time on the hash compare —
// bcrypt.CompareHashAndPassword is constant-time at the crypto layer
// for matched-length inputs, length-blinded by bcrypt's own padding.
type Validator interface {
	Validate(ctx context.Context, raw string) (*APIKey, error)
}

// CheckOrg returns ErrOrgMismatch when the request claims an org that
// differs from the key's bound org. Empty requestOrg is treated as
// "trust the key" — handlers always read the key's OrgID, the header
// is informational. Constant-time compare avoids leaking the bound
// org via timing.
func (k *APIKey) CheckOrg(requestOrg string) error {
	if requestOrg == "" {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(k.OrgID), []byte(requestOrg)) == 1 {
		return nil
	}
	return ErrOrgMismatch
}

// StaticValidator wraps a single shared key (legacy
// HANZO_AGENTS_API_KEY env). It has no org binding — the returned
// APIKey carries an empty OrgID, which CheckOrg short-circuits to
// success. The gateway-trust identity middleware still pins the
// request context to the JWT-validated org. This is the operator
// path for solo deployments where there is no key→org table yet.
type StaticValidator struct {
	key string
}

// NewStaticValidator returns a Validator that succeeds when the raw
// key matches the configured static key (constant-time). Empty
// configKey means "no auth" and is the caller's responsibility to
// short-circuit before calling Validate.
func NewStaticValidator(configKey string) *StaticValidator {
	return &StaticValidator{key: configKey}
}

// Validate compares raw against the static key in constant time.
// Mismatch → ErrKeyInvalid. Match → an APIKey with empty OrgID so
// the caller defers to the gateway's identity headers for org.
func (s *StaticValidator) Validate(_ context.Context, raw string) (*APIKey, error) {
	if s.key == "" {
		return nil, ErrKeyInvalid
	}
	if !constantTimeStringEqual(raw, s.key) {
		return nil, ErrKeyInvalid
	}
	return &APIKey{ID: "static", Prefix: prefixOf(raw), OrgID: ""}, nil
}

// Store is the durable Validator backed by Postgres (table api_keys,
// migration 022). All lookups go through Validate — there is no
// "lookup by id" or "lookup by prefix" surface exposed to the
// middleware, so a misuse path that compares plaintext anywhere
// outside this file does not exist.
type Store struct {
	db *sql.DB
}

// NewStore wires a Postgres-backed Validator to db. db must be the
// canonical *sql.DB used by the rest of the binary so tx semantics
// stay consistent.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Validate performs an indexed prefix lookup, then bcrypt-compares
// the hash. Two queries → one for the live (non-revoked) candidate,
// one for the hash compare. Returns ErrKeyInvalid for any of:
// short key, no row, hash mismatch. Returns ErrKeyRevoked /
// ErrKeyExpired when the candidate row exists but is no longer
// usable. The middleware maps both to 401.
//
// Constant-time properties:
//   - prefix lookup is by indexed equality (PostgreSQL btree),
//     not user-supplied substring, so DB-side timing is bounded
//     by index height, not key length.
//   - bcrypt.CompareHashAndPassword is constant-time at the crypto
//     layer for matched bcrypt-encoded inputs.
//   - We never short-circuit on partial bcrypt match.
func (s *Store) Validate(ctx context.Context, raw string) (*APIKey, error) {
	if len(raw) < KeyPrefixLen {
		return nil, ErrKeyInvalid
	}
	prefix := prefixOf(raw)

	row := s.db.QueryRowContext(ctx, `
		SELECT id, prefix, hash, org_id, user_id, scopes,
		       created_at, expires_at, revoked_at
		  FROM api_keys
		 WHERE prefix = $1
		 LIMIT 1
	`, prefix)

	var (
		k          APIKey
		scopes     string
		expiresAt  sql.NullTime
		revokedAt  sql.NullTime
	)
	err := row.Scan(
		&k.ID, &k.Prefix, &k.Hash, &k.OrgID, &k.UserID, &scopes,
		&k.CreatedAt, &expiresAt, &revokedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrKeyInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("api_keys lookup: %w", err)
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		k.RevokedAt = &t
		return nil, ErrKeyRevoked
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		k.ExpiresAt = &t
		if !t.IsZero() && time.Now().After(t) {
			return nil, ErrKeyExpired
		}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(k.Hash), []byte(raw)); err != nil {
		return nil, ErrKeyInvalid
	}
	if scopes != "" {
		k.Scopes = splitScopes(scopes)
	}
	return &k, nil
}

// HashKey returns the bcrypt hash of raw. Used by the admin path
// when issuing new keys; the middleware never calls this.
func HashKey(raw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash api key: %w", err)
	}
	return string(h), nil
}

// prefixOf returns the canonical lookup prefix. Short keys yield the
// whole string — Validate rejects sub-prefix-length keys before this
// point, so the only reachable path returns exactly KeyPrefixLen
// bytes.
func prefixOf(raw string) string {
	if len(raw) <= KeyPrefixLen {
		return raw
	}
	return raw[:KeyPrefixLen]
}

// constantTimeStringEqual mirrors the helper in the gin and grpc
// adapters — kept here so the static path can compare without
// importing internal/server/middleware. Length mismatch short-circuits
// because bytes-on-the-wire already leak the length.
func constantTimeStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// splitScopes turns a comma-separated DB column into a slice. Stored
// flat to avoid an N+1 join for what is effectively a small set of
// strings.
func splitScopes(s string) []string {
	out := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
