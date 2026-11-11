-- name: LockAccountForReport :one
SELECT blocked_at, delete_requested_at FROM account WHERE id = $1 FOR UPDATE;

-- name: LockReports :exec
SELECT pg_advisory_xact_lock(824331570206::bigint);

-- name: CountAccountReports :one
SELECT count(*) FILTER (WHERE created_at >= sqlc.arg(today)::timestamptz) AS daily,
       count(*) FILTER (WHERE state = 'open') AS open,
       count(*) AS retained
FROM issue_report WHERE account_id = sqlc.arg(account_id);

-- name: CountRetainedReports :one
SELECT count(*) AS retained FROM (SELECT id FROM issue_report UNION ALL SELECT report_id FROM issue_report_export) AS retained_rows;

-- name: FindOpenReport :one
SELECT * FROM issue_report WHERE account_id = $1 AND document_key = $2 AND anchor = $3 AND kind = $4 AND target = $5 AND state = 'open';

-- name: InsertReport :one
INSERT INTO issue_report (account_id, release, document_key, anchor, document_number, kind, target, suggestion)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: ListOwnReports :many
SELECT * FROM issue_report WHERE account_id = sqlc.arg(account_id)
  AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::bigint))
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(page_size);

-- name: WithdrawOwnReport :execrows
UPDATE issue_report SET state = 'withdrawn', changed_at = now()
WHERE id = $1 AND account_id = $2 AND state = 'open';

-- name: TriageGroups :many
SELECT document_key, anchor, kind, target, count(*) AS report_count,
       count(DISTINCT account_id) AS account_count, max(created_at)::timestamptz AS newest,
       array_agg(DISTINCT suggestion::text)::text[] AS suggestions
FROM issue_report
WHERE (sqlc.narg(filter_kind)::text IS NULL OR kind = sqlc.narg(filter_kind))
  AND (sqlc.narg(filter_state)::text IS NULL OR state = sqlc.narg(filter_state))
GROUP BY document_key, anchor, kind, target
ORDER BY document_key, anchor, kind, target;

-- name: TriageReports :many
SELECT * FROM issue_report WHERE document_key = $1 AND anchor = $2 AND kind = $3 AND target = $4 ORDER BY id;

-- name: DecideReports :execrows
UPDATE issue_report SET state = sqlc.arg(decision), decided_by = sqlc.arg(decided_by), changed_at = now()
WHERE state = 'open' AND sqlc.arg(decision)::text IN ('accepted', 'rejected') AND (id = ANY(sqlc.arg(ids)::bigint[]) OR
  (sqlc.narg(group_document_key)::text IS NOT NULL AND document_key = sqlc.narg(group_document_key) AND anchor = sqlc.narg(group_anchor)::text AND kind = sqlc.narg(group_kind)::text AND target = sqlc.narg(group_target)::text));

-- name: CreateExportBatch :execrows
WITH assigned AS (
    UPDATE issue_report SET export_batch = sqlc.arg(batch_id), changed_at = now()
    WHERE state = 'accepted' AND export_batch IS NULL
    RETURNING id, document_key, anchor, document_number, release, kind, target, suggestion
)
INSERT INTO issue_report_export (batch_id, report_id, document_key, anchor, document_number, release, kind, target, suggestion)
SELECT sqlc.arg(batch_id), id, document_key, anchor, document_number, release, kind, target, suggestion FROM assigned;

-- name: BatchSnapshot :many
SELECT * FROM issue_report_export WHERE batch_id = $1 ORDER BY report_id;

-- name: DeleteExpiredReports :execrows
DELETE FROM issue_report WHERE id IN (
    SELECT id FROM issue_report WHERE state <> 'open' AND changed_at < sqlc.arg(before)::timestamptz
    ORDER BY changed_at, id LIMIT sqlc.arg(batch_limit)
);

-- name: DeleteExpiredSnapshots :execrows
DELETE FROM issue_report_export WHERE (batch_id, report_id) IN (
    SELECT batch_id, report_id FROM issue_report_export WHERE created_at < sqlc.arg(before)::timestamptz
    ORDER BY created_at, batch_id, report_id LIMIT sqlc.arg(batch_limit)
);
