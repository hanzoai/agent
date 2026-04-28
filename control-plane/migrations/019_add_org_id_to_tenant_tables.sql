-- 019_add_org_id_to_tenant_tables.sql
--
-- Add the canonical org_id column to every tenant-data table so the
-- pkg/agents.OrgView pattern can scope reads/writes to a single org.
-- The column is the gateway-supplied X-Org-Id (the JWT `owner` claim
-- per ~/work/hanzo/CLAUDE.md HTTP-header convention 2026-03-27).
--
-- Backfill rules:
--   * Existing rows get org_id = '' (solo path). pkg/agents.OrgView
--     with empty org maps onto the existing unscoped queries, so
--     legacy data keeps working until it is migrated explicitly.
--   * NULL is forbidden — any new write that doesn't carry an org
--     hits the gateway-trust 401 before reaching the SQL layer.
--
-- Indexes are partial (org_id != '') so the solo-path query plan
-- stays unchanged. Composite indexes match the canonical access
-- pattern: org_id is always the leftmost predicate.

-- +goose Up
-- +goose StatementBegin

ALTER TABLE executions               ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_runs            ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_steps           ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE did_registry             ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_dids               ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE component_dids           ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE execution_vcs            ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_vcs             ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE execution_webhook_events ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_execution_events ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_run_events      ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE observability_webhooks   ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE observability_dead_letter_queue ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';

-- Composite indexes: org_id leftmost. Partial WHERE keeps the
-- solo-path index plan unchanged for legacy installs.
CREATE INDEX IF NOT EXISTS idx_executions_org_started
    ON executions(org_id, started_at DESC) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_workflow_runs_org_started
    ON workflow_runs(org_id, started_at DESC) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_workflow_steps_org_run
    ON workflow_steps(org_id, run_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_did_registry_org
    ON did_registry(org_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_agent_dids_org
    ON agent_dids(org_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_component_dids_org
    ON component_dids(org_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_execution_vcs_org
    ON execution_vcs(org_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_workflow_vcs_org
    ON workflow_vcs(org_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_execution_webhook_events_org
    ON execution_webhook_events(org_id, execution_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_workflow_execution_events_org
    ON workflow_execution_events(org_id, execution_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_workflow_run_events_org
    ON workflow_run_events(org_id, run_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_observability_webhooks_org
    ON observability_webhooks(org_id) WHERE org_id != '';
CREATE INDEX IF NOT EXISTS idx_observability_dlq_org
    ON observability_dead_letter_queue(org_id) WHERE org_id != '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_observability_dlq_org;
DROP INDEX IF EXISTS idx_observability_webhooks_org;
DROP INDEX IF EXISTS idx_workflow_run_events_org;
DROP INDEX IF EXISTS idx_workflow_execution_events_org;
DROP INDEX IF EXISTS idx_execution_webhook_events_org;
DROP INDEX IF EXISTS idx_workflow_vcs_org;
DROP INDEX IF EXISTS idx_execution_vcs_org;
DROP INDEX IF EXISTS idx_component_dids_org;
DROP INDEX IF EXISTS idx_agent_dids_org;
DROP INDEX IF EXISTS idx_did_registry_org;
DROP INDEX IF EXISTS idx_workflow_steps_org_run;
DROP INDEX IF EXISTS idx_workflow_runs_org_started;
DROP INDEX IF EXISTS idx_executions_org_started;

-- SQLite does not support DROP COLUMN; for PostgreSQL the column
-- drops are explicit. Forward-only migrations are the canonical
-- Hanzo path, so this is a safety net rather than a routine
-- rollback.
ALTER TABLE observability_dead_letter_queue DROP COLUMN IF EXISTS org_id;
ALTER TABLE observability_webhooks   DROP COLUMN IF EXISTS org_id;
ALTER TABLE workflow_run_events      DROP COLUMN IF EXISTS org_id;
ALTER TABLE workflow_execution_events DROP COLUMN IF EXISTS org_id;
ALTER TABLE execution_webhook_events DROP COLUMN IF EXISTS org_id;
ALTER TABLE workflow_vcs             DROP COLUMN IF EXISTS org_id;
ALTER TABLE execution_vcs            DROP COLUMN IF EXISTS org_id;
ALTER TABLE component_dids           DROP COLUMN IF EXISTS org_id;
ALTER TABLE agent_dids               DROP COLUMN IF EXISTS org_id;
ALTER TABLE did_registry             DROP COLUMN IF EXISTS org_id;
ALTER TABLE workflow_steps           DROP COLUMN IF EXISTS org_id;
ALTER TABLE workflow_runs            DROP COLUMN IF EXISTS org_id;
ALTER TABLE executions               DROP COLUMN IF EXISTS org_id;

-- +goose StatementEnd
