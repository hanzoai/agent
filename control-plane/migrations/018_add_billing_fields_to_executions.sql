-- +goose Up
-- +goose StatementBegin
ALTER TABLE executions ADD COLUMN billing_user_id TEXT;
ALTER TABLE executions ADD COLUMN debit_transaction_id TEXT;
ALTER TABLE executions ADD COLUMN actual_cost_cents BIGINT;

-- Index for querying executions by billing user (usage history)
CREATE INDEX IF NOT EXISTS idx_executions_billing_user_id ON executions(billing_user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_executions_billing_user_id;
-- SQLite does not support DROP COLUMN natively; for PostgreSQL:
-- ALTER TABLE executions DROP COLUMN IF EXISTS billing_user_id;
-- ALTER TABLE executions DROP COLUMN IF EXISTS debit_transaction_id;
-- ALTER TABLE executions DROP COLUMN IF EXISTS actual_cost_cents;
-- +goose StatementEnd
