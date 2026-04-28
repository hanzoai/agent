-- 020_backfill_org_id.sql
--
-- Backfill org_id for rows created BEFORE migration 019 set the
-- column. Those rows currently carry org_id = '' (the 019 DEFAULT).
-- This migration assigns them to the reserved sentinel org
-- `_legacy`, which:
--
--   * Is NOT a valid IAM org slug (leading underscore is reserved
--     in IAM owner-claim spec, see ~/work/hanzo/CLAUDE.md
--     2026-03-27 HTTP-header convention).
--   * Cannot be set by any caller — RequireIdentity rejects empty,
--     and the gateway maps real owner claims (no leading '_').
--   * Lets the SQL layer enforce non-empty org_id (migration 021's
--     CHECK constraint) without losing legacy data.
--
-- Per-table mapping decision:
--
--   executions, workflow_runs, workflow_steps, workflow_*_events,
--   execution_webhook_events, observability_*:
--     No reliable created_by/owner column exists on these tables.
--     Legacy rows are operator-shared until explicit migration via
--     `agentctl migrate-legacy --to <org>`.
--
--   did_registry, agent_dids, component_dids, execution_vcs,
--   workflow_vcs:
--     Same — no owner column. The DID/VC graph predates multitenant.
--     Operators must re-anchor these via the DID service after
--     assigning a real org.
--
-- This migration is IDEMPOTENT and SAFE to re-run: it only touches
-- rows where org_id = '' (the 019 default sentinel).

-- +goose Up
-- +goose StatementBegin

UPDATE executions                   SET org_id = '_legacy' WHERE org_id = '';
UPDATE workflow_runs                SET org_id = '_legacy' WHERE org_id = '';
UPDATE workflow_steps               SET org_id = '_legacy' WHERE org_id = '';
UPDATE did_registry                 SET org_id = '_legacy' WHERE org_id = '';
UPDATE agent_dids                   SET org_id = '_legacy' WHERE org_id = '';
UPDATE component_dids               SET org_id = '_legacy' WHERE org_id = '';
UPDATE execution_vcs                SET org_id = '_legacy' WHERE org_id = '';
UPDATE workflow_vcs                 SET org_id = '_legacy' WHERE org_id = '';
UPDATE execution_webhook_events     SET org_id = '_legacy' WHERE org_id = '';
UPDATE workflow_execution_events    SET org_id = '_legacy' WHERE org_id = '';
UPDATE workflow_run_events          SET org_id = '_legacy' WHERE org_id = '';
UPDATE observability_webhooks       SET org_id = '_legacy' WHERE org_id = '';
UPDATE observability_dead_letter_queue SET org_id = '_legacy' WHERE org_id = '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Reverse only the sentinel assignment. Real org_id values stay.
UPDATE observability_dead_letter_queue SET org_id = '' WHERE org_id = '_legacy';
UPDATE observability_webhooks       SET org_id = '' WHERE org_id = '_legacy';
UPDATE workflow_run_events          SET org_id = '' WHERE org_id = '_legacy';
UPDATE workflow_execution_events    SET org_id = '' WHERE org_id = '_legacy';
UPDATE execution_webhook_events     SET org_id = '' WHERE org_id = '_legacy';
UPDATE workflow_vcs                 SET org_id = '' WHERE org_id = '_legacy';
UPDATE execution_vcs                SET org_id = '' WHERE org_id = '_legacy';
UPDATE component_dids               SET org_id = '' WHERE org_id = '_legacy';
UPDATE agent_dids                   SET org_id = '' WHERE org_id = '_legacy';
UPDATE did_registry                 SET org_id = '' WHERE org_id = '_legacy';
UPDATE workflow_steps               SET org_id = '' WHERE org_id = '_legacy';
UPDATE workflow_runs                SET org_id = '' WHERE org_id = '_legacy';
UPDATE executions                   SET org_id = '' WHERE org_id = '_legacy';

-- +goose StatementEnd
