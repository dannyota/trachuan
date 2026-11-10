-- name: InsertFetchObservation :exec
INSERT INTO run_fetch_observation (run_id, item_key, generation, attempt, resource_key, artifact_sha256, source, path, url, content_sha256, size, acquisition, fetched_at, origin_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (run_id, item_key, generation, attempt, resource_key) DO NOTHING;

-- name: ListItemFetchObservations :many
SELECT * FROM run_fetch_observation
WHERE run_id = $1 AND item_key = $2 AND generation = $3 AND attempt = $4
ORDER BY resource_key COLLATE "C";

-- name: SetFetchedFileTime :execrows
UPDATE fetched_file SET fetched_at = sqlc.narg(fetched_at)
WHERE artifact_id = sqlc.arg(artifact_id) AND source = sqlc.arg(source) AND name = sqlc.arg(name)
  AND path = sqlc.arg(path) AND sha256 = sqlc.arg(sha256) AND size = sqlc.arg(size) AND fetched_at IS NULL;

-- name: GetRunItemCommitted :one
SELECT run_item.state, run_item.generation, run_item.attempt, run_item.message, artifact.sha256 AS artifact_sha256
FROM run_item LEFT JOIN artifact ON artifact.id = run_item.artifact_id
WHERE run_item.run_id = $1 AND run_item.stage = $2 AND run_item.item_key = $3
FOR UPDATE OF run_item;
