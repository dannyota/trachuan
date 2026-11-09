-- name: GetRunGeneration :one
SELECT generation FROM run WHERE id = $1;

-- name: GetRunGenerationForShare :one
SELECT generation FROM run WHERE id = $1 FOR SHARE;
