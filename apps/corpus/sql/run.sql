-- name: GetRunGeneration :one
SELECT generation FROM run WHERE id = $1;

-- name: GetRunGenerationForShare :one
SELECT generation FROM run WHERE id = $1 FOR SHARE;

-- name: CountIncompleteItems :one
SELECT count(*) FROM run_item WHERE run_id = $1 AND state IN ('pending', 'failed');
