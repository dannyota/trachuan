-- name: InsertArtifact :one
INSERT INTO artifact (stage, item_key, input_hash, recipe_hash, path, sha256)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (stage, input_hash, recipe_hash) DO NOTHING
RETURNING id;

-- name: GetArtifactID :one
SELECT id FROM artifact WHERE stage = $1 AND input_hash = $2 AND recipe_hash = $3;
