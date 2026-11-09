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

-- name: EnsureRunSource :exec
INSERT INTO run_source (run_id, source, state) VALUES ($1, $2, 'active')
ON CONFLICT (run_id, source) DO NOTHING;

-- name: GetRunSourceForShare :one
SELECT * FROM run_source WHERE run_id = $1 AND source = $2 FOR SHARE;

-- name: GetRunSourceForUpdate :one
SELECT * FROM run_source WHERE run_id = $1 AND source = $2 FOR UPDATE;

-- name: PauseRunSource :execrows
UPDATE run_source SET state = 'source_paused', pause_reason = sqlc.arg(reason), evidence_ref = sqlc.narg(evidence_ref), paused_at = now(), cleared_at = NULL
WHERE run_id = sqlc.arg(run_id) AND source = sqlc.arg(source) AND state = 'active';

-- name: ClearRunSource :execrows
UPDATE run_source SET state = 'active', cleared_at = now()
WHERE run_id = $1 AND source = $2 AND state = 'source_paused';

-- name: InsertRunSourceEvent :exec
INSERT INTO run_source_event (run_id, source, event, reason, evidence_ref, clear_note)
VALUES (sqlc.arg(run_id), sqlc.arg(source), sqlc.arg(event), sqlc.narg(reason), sqlc.narg(evidence_ref), sqlc.narg(clear_note));

-- name: ListRunSources :many
SELECT * FROM run_source WHERE run_id = $1 ORDER BY source COLLATE "C";

-- name: CountPausedSources :one
SELECT count(*) FROM run_source WHERE run_id = $1 AND state = 'source_paused';

-- name: SetFetchCloseInputHash :execrows
UPDATE run_scope SET fetch_close_input_hash = $2 WHERE run_id = $1 AND (fetch_close_input_hash IS NULL OR fetch_close_input_hash = $2);
