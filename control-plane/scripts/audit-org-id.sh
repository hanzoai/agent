#!/usr/bin/env bash
# audit-org-id.sh
#
# Reports the count of rows with org_id='' per tenant table. This is
# the audit step in the deploy sequence documented in
# migrations/MIGRATION_ORDER.md:
#
#   (a) deploy 019  ← schema in place, default ''
#   (b) run THIS script ← see what's empty
#   (c) deploy 020 backfill ← assign sentinel org per table
#   (d) re-run THIS script ← verify zero ''
#   (e) deploy 021 NOT NULL  ← lock it down
#   (f) flip AGENTD_REQUIRE_IDENTITY=true
#
# Usage:
#   AGENTD_POSTGRES_DSN="postgres://..." ./scripts/audit-org-id.sh
#
# Exit code is the total number of empty-string rows across all
# tables. Zero means it's safe to apply migration 021.

set -euo pipefail

DSN="${AGENTD_POSTGRES_DSN:-${HANZO_AGENTS_STORAGE_POSTGRES_URL:-}}"
if [[ -z "${DSN}" ]]; then
    echo "ERROR: set AGENTD_POSTGRES_DSN or HANZO_AGENTS_STORAGE_POSTGRES_URL" >&2
    exit 2
fi

TABLES=(
    executions
    workflow_runs
    workflow_steps
    did_registry
    agent_dids
    component_dids
    execution_vcs
    workflow_vcs
    execution_webhook_events
    workflow_execution_events
    workflow_run_events
    observability_webhooks
    observability_dead_letter_queue
)

printf '%-40s %12s %12s %12s\n' 'table' 'total' 'empty_org' 'pct_empty'
printf '%s\n' '----------------------------------------------------------------------------'

total_empty=0
for t in "${TABLES[@]}"; do
    # information_schema check first — skip tables that don't exist
    # in this database (e.g. partial deploys).
    exists=$(psql "${DSN}" -tAc "SELECT 1 FROM information_schema.tables WHERE table_name='${t}'" || echo '')
    if [[ -z "${exists}" ]]; then
        printf '%-40s %12s %12s %12s\n' "${t}" 'n/a' 'n/a' 'skip'
        continue
    fi

    total=$(psql "${DSN}" -tAc "SELECT COUNT(*) FROM ${t}" 2>/dev/null || echo '0')
    empty=$(psql "${DSN}" -tAc "SELECT COUNT(*) FROM ${t} WHERE org_id = ''" 2>/dev/null || echo '0')

    if [[ "${total}" == "0" ]]; then
        pct='—'
    else
        pct=$(awk -v e="${empty}" -v t="${total}" 'BEGIN{printf "%.1f%%", (e/t)*100}')
    fi

    printf '%-40s %12s %12s %12s\n' "${t}" "${total}" "${empty}" "${pct}"
    total_empty=$((total_empty + empty))
done

printf '%s\n' '----------------------------------------------------------------------------'
echo "TOTAL EMPTY org_id ROWS: ${total_empty}"

if [[ "${total_empty}" -gt 0 ]]; then
    echo ""
    echo "ACTION: deploy migrations/020_backfill_org_id.sql, then re-run."
    echo "DO NOT deploy migrations/021_org_id_not_null.sql until this is 0."
fi

exit "${total_empty}"
