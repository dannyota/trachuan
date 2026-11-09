-- name: InsertGap :exec
INSERT INTO gap (run_id, stage, item_key, kind, detail, source, origin, cause, endpoint, http_status, evidence_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (run_id, stage, item_key, kind, detail) DO NOTHING;

-- name: ListRunGaps :many
SELECT gap.* FROM gap JOIN run ON run.id = gap.run_id
WHERE run.country = $1 AND run.rel = $2 ORDER BY gap.stage, gap.item_key, gap.kind, gap.detail;

-- name: AcceptRunGaps :execrows
UPDATE gap SET accepted_at = now(), note = sqlc.arg(note)
WHERE run_id = (SELECT id FROM run WHERE country = sqlc.arg(country) AND rel = sqlc.arg(rel))
  AND kind = sqlc.arg(kind) AND accepted_at IS NULL;

-- name: CountUnacceptedGaps :one
SELECT count(*) FROM gap WHERE run_id = $1 AND accepted_at IS NULL;
