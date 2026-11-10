-- name: ListChunkInputs :many
SELECT e.item_key, fa.path AS fetch_path, fa.sha256 AS fetch_sha256,
       ea.path AS extract_path, ea.sha256 AS extract_sha256,
       ea.input_hash AS extract_input_hash, fa.item_key AS fetch_key,
       ea.item_key AS extract_key
FROM run_item e
JOIN artifact ea ON ea.id = e.artifact_id
JOIN run_item f ON f.run_id = e.run_id AND f.item_key = e.item_key
    AND f.stage = 'fetch' AND f.state = 'done'
JOIN artifact fa ON fa.id = f.artifact_id
WHERE e.run_id = $1 AND e.stage = 'extract' AND e.state = 'done'
ORDER BY e.item_key;

-- name: GetChunkInput :one
SELECT fa.path AS fetch_path, fa.sha256 AS fetch_sha256,
       ea.path AS extract_path, ea.sha256 AS extract_sha256,
       ea.input_hash AS extract_input_hash, fa.item_key AS fetch_key,
       ea.item_key AS extract_key
FROM run_item e
JOIN artifact ea ON ea.id = e.artifact_id
JOIN run_item f ON f.run_id = e.run_id AND f.item_key = e.item_key
    AND f.stage = 'fetch' AND f.state = 'done'
JOIN artifact fa ON fa.id = f.artifact_id
WHERE e.run_id = $1 AND e.item_key = $2 AND e.stage = 'extract' AND e.state = 'done';

-- name: GetExtractArtifactByHash :one
SELECT id FROM artifact WHERE stage = 'extract' AND sha256 = $1 AND item_key = $2 ORDER BY id LIMIT 1;

-- name: InsertChunk :exec
INSERT INTO chunk (artifact_id, ordinal, extract_artifact_id, node_id, anchor, context_prefix, content, rough_tokens, spans)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListChunks :many
SELECT * FROM chunk WHERE artifact_id = $1 ORDER BY ordinal;
