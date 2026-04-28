# Migration Deploy Order — org_id rollout

Migrations 019, 020, 021 must be deployed in lockstep with the
`AGENTD_REQUIRE_IDENTITY` flag flip. Out-of-order deploy makes the
solo path cross-tenant readable. Follow the steps exactly.

## Pre-condition

`AGENTD_REQUIRE_IDENTITY` is `false` (or unset) in production. If it
is already `true`, stop and revert to `false` before proceeding —
otherwise step (b) will fail in flight.

## Steps

| # | Action | Verify |
|---|--------|--------|
| a | Apply `019_add_org_id_to_tenant_tables.sql`. The column is added with `DEFAULT ''` so every legacy row has a placeholder. | `psql -c "\d executions"` shows `org_id text NOT NULL DEFAULT ''::text`. |
| b | Run `scripts/audit-org-id.sh`. Capture the row counts per table. | Exit code = number of empty-string rows. Non-zero is expected here. |
| c | Apply `020_backfill_org_id.sql`. Empty-string rows are reassigned to the sentinel org `_legacy`. | The migration is idempotent. |
| d | Re-run `scripts/audit-org-id.sh`. | Exit code MUST be `0`. If non-zero, do not proceed. |
| e | Apply `021_org_id_not_null.sql`. The DEFAULT '' clause is dropped and a `CHECK (org_id <> '')` constraint is added. | `psql -c "\d+ executions"` shows the constraint VALIDATED. |
| f | Flip `AGENTD_REQUIRE_IDENTITY=true` in helm values and roll the deployment. | New requests without identity headers return 401. |

## Rationale

* **019 first**: schema must exist before code can write the column.
* **020 before 021**: 021's CHECK constraint refuses the empty string,
  so any row left behind after 019 would fail validation. 020
  guarantees zero empty rows.
* **021 before flag flip**: the SQL constraint is the last line of
  defense. If a handler bug skipped middleware, the INSERT must fail
  rather than land in the unscoped bucket.
* **Flag flip last**: the middleware enforces non-empty org_id at the
  HTTP layer once the SQL layer can no longer accept empty values.

## Helm enforcement

`deployments/helm/hanzo-agents/templates/migration-job.yaml` runs 019,
020, 021 as a `post-install,post-upgrade` Helm hook in that order. The
hook fails fast if 020 reports any empty rows after running, blocking
021 from running and the chart from upgrading.

## Legacy rows

Rows assigned `org_id='_legacy'` are operator-shared. They are
**read-only** under the standard `pkg/agents.OrgView` (which scopes by
the request's org_id, never `_legacy`). To migrate them to a real org,
use the operator CLI:

```bash
agentctl migrate-legacy --table executions --to <real-org-id>
```

`_legacy` is not a valid IAM owner claim (leading underscore is
reserved per `~/work/hanzo/CLAUDE.md` 2026-03-27 HTTP-header
convention), so no real request can ever produce or read it.
