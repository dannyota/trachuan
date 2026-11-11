-- name: UpsertFetchBaseline :exec
INSERT INTO fetch_baseline (document_key, fetch_recipe_hash, artifact_id, run_id, item_key, generation, attempt)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (document_key, fetch_recipe_hash)
DO UPDATE SET artifact_id = EXCLUDED.artifact_id, run_id = EXCLUDED.run_id,
    item_key = EXCLUDED.item_key, generation = EXCLUDED.generation,
    attempt = EXCLUDED.attempt, recorded_at = now();

-- name: GetFetchBaseline :one
SELECT b.document_key, b.fetch_recipe_hash, b.artifact_id, b.run_id, b.item_key,
       b.generation, b.attempt, b.recorded_at, a.path, a.sha256
FROM fetch_baseline b JOIN artifact a ON a.id = b.artifact_id
WHERE b.document_key = $1 AND b.fetch_recipe_hash = $2;
