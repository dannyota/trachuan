-- name: InsertEmbedRow :execrows
INSERT INTO embed_row (recipe_hash, input_sha256, artifact_id, row, token_count)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (recipe_hash, input_sha256) DO NOTHING;

-- name: LookupEmbedRows :many
SELECT e.input_sha256, e.row, e.token_count, a.id AS artifact_id, a.input_hash, a.path, a.sha256
FROM embed_row e JOIN artifact a ON a.id = e.artifact_id
WHERE e.recipe_hash = sqlc.arg(recipe_hash) AND e.input_sha256 = ANY(sqlc.arg(input_hashes)::text[]);

-- name: ListEmbedArtifacts :many
SELECT id, input_hash, path, sha256 FROM artifact
WHERE stage = 'embed' AND recipe_hash = $1 ORDER BY id;
