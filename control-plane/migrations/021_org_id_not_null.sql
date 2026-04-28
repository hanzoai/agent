-- 021_org_id_not_null.sql
--
-- Lock down org_id: drop the DEFAULT '' clause and add a CHECK
-- constraint that bars empty strings. The combination of (NOT NULL,
-- no DEFAULT, CHECK <> '') makes the SQL layer the last line of
-- defense — even if a handler bug skipped the auth middleware, the
-- INSERT would fail rather than land in the legacy unscoped bucket.
--
-- PRE-CONDITION: scripts/audit-org-id.sh reports zero empty rows.
-- The deploy order in migrations/MIGRATION_ORDER.md must be
-- followed: this migration WILL fail if run before 020 in a DB
-- that has any legacy rows.
--
-- PostgreSQL: NOT VALID + VALIDATE CONSTRAINT pattern minimises
-- table lock duration on large tables.

-- +goose Up
-- +goose StatementBegin

ALTER TABLE executions                   ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE workflow_runs                ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE workflow_steps               ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE did_registry                 ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE agent_dids                   ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE component_dids               ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE execution_vcs                ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE workflow_vcs                 ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE execution_webhook_events     ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE workflow_execution_events    ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE workflow_run_events          ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE observability_webhooks       ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE observability_dead_letter_queue ALTER COLUMN org_id DROP DEFAULT;

ALTER TABLE executions                   ADD CONSTRAINT executions_org_id_nonempty                   CHECK (org_id <> '') NOT VALID;
ALTER TABLE workflow_runs                ADD CONSTRAINT workflow_runs_org_id_nonempty                CHECK (org_id <> '') NOT VALID;
ALTER TABLE workflow_steps               ADD CONSTRAINT workflow_steps_org_id_nonempty               CHECK (org_id <> '') NOT VALID;
ALTER TABLE did_registry                 ADD CONSTRAINT did_registry_org_id_nonempty                 CHECK (org_id <> '') NOT VALID;
ALTER TABLE agent_dids                   ADD CONSTRAINT agent_dids_org_id_nonempty                   CHECK (org_id <> '') NOT VALID;
ALTER TABLE component_dids               ADD CONSTRAINT component_dids_org_id_nonempty               CHECK (org_id <> '') NOT VALID;
ALTER TABLE execution_vcs                ADD CONSTRAINT execution_vcs_org_id_nonempty                CHECK (org_id <> '') NOT VALID;
ALTER TABLE workflow_vcs                 ADD CONSTRAINT workflow_vcs_org_id_nonempty                 CHECK (org_id <> '') NOT VALID;
ALTER TABLE execution_webhook_events     ADD CONSTRAINT execution_webhook_events_org_id_nonempty     CHECK (org_id <> '') NOT VALID;
ALTER TABLE workflow_execution_events    ADD CONSTRAINT workflow_execution_events_org_id_nonempty    CHECK (org_id <> '') NOT VALID;
ALTER TABLE workflow_run_events          ADD CONSTRAINT workflow_run_events_org_id_nonempty          CHECK (org_id <> '') NOT VALID;
ALTER TABLE observability_webhooks       ADD CONSTRAINT observability_webhooks_org_id_nonempty       CHECK (org_id <> '') NOT VALID;
ALTER TABLE observability_dead_letter_queue ADD CONSTRAINT observability_dead_letter_queue_org_id_nonempty CHECK (org_id <> '') NOT VALID;

ALTER TABLE executions                   VALIDATE CONSTRAINT executions_org_id_nonempty;
ALTER TABLE workflow_runs                VALIDATE CONSTRAINT workflow_runs_org_id_nonempty;
ALTER TABLE workflow_steps               VALIDATE CONSTRAINT workflow_steps_org_id_nonempty;
ALTER TABLE did_registry                 VALIDATE CONSTRAINT did_registry_org_id_nonempty;
ALTER TABLE agent_dids                   VALIDATE CONSTRAINT agent_dids_org_id_nonempty;
ALTER TABLE component_dids               VALIDATE CONSTRAINT component_dids_org_id_nonempty;
ALTER TABLE execution_vcs                VALIDATE CONSTRAINT execution_vcs_org_id_nonempty;
ALTER TABLE workflow_vcs                 VALIDATE CONSTRAINT workflow_vcs_org_id_nonempty;
ALTER TABLE execution_webhook_events     VALIDATE CONSTRAINT execution_webhook_events_org_id_nonempty;
ALTER TABLE workflow_execution_events    VALIDATE CONSTRAINT workflow_execution_events_org_id_nonempty;
ALTER TABLE workflow_run_events          VALIDATE CONSTRAINT workflow_run_events_org_id_nonempty;
ALTER TABLE observability_webhooks       VALIDATE CONSTRAINT observability_webhooks_org_id_nonempty;
ALTER TABLE observability_dead_letter_queue VALIDATE CONSTRAINT observability_dead_letter_queue_org_id_nonempty;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE observability_dead_letter_queue DROP CONSTRAINT IF EXISTS observability_dead_letter_queue_org_id_nonempty;
ALTER TABLE observability_webhooks       DROP CONSTRAINT IF EXISTS observability_webhooks_org_id_nonempty;
ALTER TABLE workflow_run_events          DROP CONSTRAINT IF EXISTS workflow_run_events_org_id_nonempty;
ALTER TABLE workflow_execution_events    DROP CONSTRAINT IF EXISTS workflow_execution_events_org_id_nonempty;
ALTER TABLE execution_webhook_events     DROP CONSTRAINT IF EXISTS execution_webhook_events_org_id_nonempty;
ALTER TABLE workflow_vcs                 DROP CONSTRAINT IF EXISTS workflow_vcs_org_id_nonempty;
ALTER TABLE execution_vcs                DROP CONSTRAINT IF EXISTS execution_vcs_org_id_nonempty;
ALTER TABLE component_dids               DROP CONSTRAINT IF EXISTS component_dids_org_id_nonempty;
ALTER TABLE agent_dids                   DROP CONSTRAINT IF EXISTS agent_dids_org_id_nonempty;
ALTER TABLE did_registry                 DROP CONSTRAINT IF EXISTS did_registry_org_id_nonempty;
ALTER TABLE workflow_steps               DROP CONSTRAINT IF EXISTS workflow_steps_org_id_nonempty;
ALTER TABLE workflow_runs                DROP CONSTRAINT IF EXISTS workflow_runs_org_id_nonempty;
ALTER TABLE executions                   DROP CONSTRAINT IF EXISTS executions_org_id_nonempty;

ALTER TABLE observability_dead_letter_queue ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE observability_webhooks       ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE workflow_run_events          ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE workflow_execution_events    ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE execution_webhook_events     ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE workflow_vcs                 ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE execution_vcs                ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE component_dids               ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE agent_dids                   ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE did_registry                 ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE workflow_steps               ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE workflow_runs                ALTER COLUMN org_id SET DEFAULT '';
ALTER TABLE executions                   ALTER COLUMN org_id SET DEFAULT '';

-- +goose StatementEnd
