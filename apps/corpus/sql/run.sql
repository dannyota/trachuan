-- name: GetRunGeneration :one
SELECT generation FROM run WHERE id = $1;

-- name: GetRunGenerationForShare :one
SELECT generation FROM run WHERE id = $1 FOR SHARE;

-- name: CountIncompleteItems :one
SELECT count(*) FROM run_item WHERE run_id = $1 AND state IN ('pending', 'failed');

-- name: CreateRun :exec
INSERT INTO run (country, rel, state, snapshot) VALUES ($1, $2, 'failed', $3)
ON CONFLICT (country, rel) DO NOTHING;

-- name: GetRunForUpdate :one
SELECT * FROM run WHERE country = $1 AND rel = $2 FOR UPDATE;

-- name: GetRun :one
SELECT * FROM run WHERE country = $1 AND rel = $2;

-- name: StartRun :exec
UPDATE run SET generation = generation + 1, state = 'starting', next_stage = $2,
    until_stage = $3, workflow_id = $4, snapshot = $5, message = NULL, updated_at = now()
WHERE id = $1;

-- name: ResetFailedItems :exec
UPDATE run_item SET state = 'pending', updated_at = now()
WHERE run_id = $1 AND stage = $2 AND state = 'failed';

-- name: SetRunState :exec
UPDATE run SET state = $2, message = $3, updated_at = now() WHERE id = $1;

-- name: RecoverRun :exec
UPDATE run SET generation = generation + 1, state = 'failed', message = 'recovered', updated_at = now()
WHERE id = $1;

-- name: ListRunItemCounts :many
SELECT stage, state, count(*) AS count FROM run_item WHERE run_id = $1 GROUP BY stage, state ORDER BY stage, state;

-- name: ListUnacceptedGapCounts :many
SELECT stage, kind, count(*) AS count FROM gap WHERE run_id = $1 AND accepted_at IS NULL GROUP BY stage, kind ORDER BY stage, kind;

-- name: GetBatchRun :one
SELECT * FROM run WHERE id = $1;

-- name: GetBatchRunState :one
SELECT state FROM run WHERE id = $1;

-- name: GetControlRunForUpdate :one
SELECT * FROM run WHERE id = $1 FOR NO KEY UPDATE;

-- name: AdvanceRunStage :exec
UPDATE run SET next_stage = sqlc.narg(next_stage), updated_at = now() WHERE id = sqlc.arg(id);

-- name: MarkRunFromStates :execrows
UPDATE run SET state = sqlc.arg(state), message = sqlc.narg(message), updated_at = now()
WHERE id = sqlc.arg(id) AND generation = sqlc.arg(generation) AND state = ANY(sqlc.arg(allowed_states)::text[]);
