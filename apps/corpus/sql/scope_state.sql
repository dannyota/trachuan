-- name: EnsureRunScope :exec
INSERT INTO run_scope (run_id) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: GetRunScope :one
SELECT * FROM run_scope WHERE run_id = $1;

-- name: PublishSeeds :execrows
UPDATE run_scope SET seeds_snapshot = $2, seeds_hash = $3 WHERE run_id = $1 AND (seeds_snapshot IS NULL OR (seeds_snapshot = $2 AND seeds_hash = $3));

-- name: PublishExpanded :execrows
UPDATE run_scope SET expanded_snapshot = $2, expanded_hash = $3 WHERE run_id = $1 AND (expanded_snapshot IS NULL OR (expanded_snapshot = $2 AND expanded_hash = $3));

-- name: PublishFinal :execrows
UPDATE run_scope SET final_snapshot = $2, final_hash = $3 WHERE run_id = $1 AND (final_snapshot IS NULL OR (final_snapshot = $2 AND final_hash = $3));

-- name: ClearSeeds :exec
UPDATE run_scope SET seeds_snapshot = NULL, seeds_hash = NULL WHERE run_id = $1;

-- name: ClearExpanded :exec
UPDATE run_scope SET expanded_snapshot = NULL, expanded_hash = NULL WHERE run_id = $1;

-- name: ClearFinal :exec
UPDATE run_scope SET final_snapshot = NULL, final_hash = NULL, fetch_close_input_hash = NULL WHERE run_id = $1;

-- name: InsertRunFailure :exec
INSERT INTO run_failure (run_id, failure_identity, code, stage, item_key, reason, evidence_refs)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (run_id, failure_identity) DO NOTHING;

-- name: ListRunFailures :many
SELECT * FROM run_failure WHERE run_id = $1 ORDER BY failure_identity COLLATE "C";

-- name: ListBlockingRunFailureCodes :many
SELECT DISTINCT code FROM run_failure
WHERE run_id = sqlc.arg(run_id) AND code = ANY(sqlc.arg(codes)::text[])
ORDER BY code;

-- name: ListReleaseBlockingFailureCodes :many
SELECT DISTINCT run_failure.code FROM run_failure JOIN run ON run.id = run_failure.run_id
WHERE run.country = sqlc.arg(country) AND run.rel = sqlc.arg(rel) AND run_failure.code = ANY(sqlc.arg(codes)::text[])
ORDER BY run_failure.code;
