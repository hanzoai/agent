-- 022_api_keys_table.sql
--
-- Bound API keys: the key value itself carries the org_id. APIKeyAuth
-- loads the key, reads OrgID, and rejects (403) when the request's
-- X-Org-Id mismatches. Closes Red 2026-04-27 P0-2: a valid X-API-Key
-- combined with attacker-chosen X-Org-Id no longer crosses tenants.
--
-- Hash column stores bcrypt of the raw key (golang.org/x/crypto/bcrypt,
-- DefaultCost). Prefix column is the first 11 bytes of the raw key
-- ("hk-" + 8 hex) — enough entropy to make prefix-only enumeration
-- pointless and short enough for an indexed lookup.
--
-- Lookup path: SELECT WHERE prefix = $1 AND revoked_at IS NULL
--              AND (expires_at IS NULL OR expires_at > now())
-- → bcrypt.CompareHashAndPassword(hash, raw)  (constant-time at the
--   crypto layer, length-blinded by bcrypt itself).
--
-- org_id NOT NULL is the schema-level guarantee that no row can land
-- in the legacy unscoped bucket. revoked_at gets a partial index for
-- the live-key fast path; expires_at is read on every Validate call
-- so it stays unindexed (cheap comparison).

-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    prefix      TEXT NOT NULL,
    hash        TEXT NOT NULL,
    org_id      TEXT NOT NULL CHECK (org_id <> ''),
    user_id     TEXT NOT NULL CHECK (user_id <> ''),
    scopes      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_prefix_live
    ON api_keys(prefix) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_org
    ON api_keys(org_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_user
    ON api_keys(user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_api_keys_user;
DROP INDEX IF EXISTS idx_api_keys_org;
DROP INDEX IF EXISTS idx_api_keys_prefix_live;
DROP TABLE IF EXISTS api_keys;

-- +goose StatementEnd
