-- name: GetPinnedArtifact :one
SELECT r.id AS run_id, i.generation, i.attempt, a.stage, a.path, a.sha256, a.input_hash, a.recipe_hash
FROM run r JOIN run_item i ON i.run_id = r.id JOIN artifact a ON a.id = i.artifact_id
WHERE r.country = sqlc.arg(country) AND r.rel = sqlc.arg(rel)
  AND i.stage = sqlc.arg(stage) AND i.item_key = sqlc.arg(item_key) AND i.state = 'done'
  AND a.stage = i.stage AND a.sha256 = sqlc.arg(sha256);

-- name: ListDocumentAcquisitions :many
SELECT r.rel, i.stage, a.path, a.sha256
FROM run r JOIN run_item i ON i.run_id = r.id JOIN artifact a ON a.id = i.artifact_id
WHERE r.country = sqlc.arg(country) AND i.item_key = sqlc.arg(item_key)
  AND i.state = 'done' AND i.stage IN ('expand', 'fetch') AND a.stage = i.stage;
