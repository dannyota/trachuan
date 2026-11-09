-- name: ListEmbedInputs :many
SELECT i.item_key, a.sha256
FROM run_item i
JOIN artifact a ON a.id = i.artifact_id
WHERE i.run_id = $1 AND i.stage = 'chunk' AND i.state = 'done'
  AND a.stage = 'chunk' AND a.item_key = i.item_key
ORDER BY i.item_key;

-- name: GetEmbedInput :one
SELECT a.path, a.sha256
FROM run_item i
JOIN artifact a ON a.id = i.artifact_id
WHERE i.run_id = $1 AND i.item_key = $2 AND i.stage = 'chunk' AND i.state = 'done'
  AND a.stage = 'chunk' AND a.item_key = i.item_key;
