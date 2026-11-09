-- name: InsertRunItem :exec
INSERT INTO run_item (run_id, stage, item_key, input_hash, recipe_hash, state, batch)
VALUES ($1, $2, $3, $4, $5, 'pending', $6)
ON CONFLICT (run_id, stage, item_key) DO NOTHING;

-- name: UpdateRunItemFenced :execrows
UPDATE run_item AS i
SET input_hash = sqlc.narg(input_hash), recipe_hash = sqlc.narg(recipe_hash),
    generation = sqlc.arg(generation), attempt = sqlc.arg(attempt),
    artifact_id = sqlc.narg(artifact_id), state = sqlc.arg(state),
    message = sqlc.narg(message), updated_at = now()
FROM run AS r
WHERE i.run_id = r.id AND i.run_id = sqlc.arg(run_id)
  AND i.stage = sqlc.arg(stage) AND i.item_key = sqlc.arg(item_key)
  AND r.generation = sqlc.arg(generation)
  AND (i.generation, i.attempt) <= (sqlc.arg(generation), sqlc.arg(attempt));

-- name: ListPendingBatchItems :many
SELECT * FROM run_item
WHERE run_id = $1 AND stage = $2 AND batch = $3 AND state = 'pending'
ORDER BY item_key;

-- name: InsertPlannedItem :exec
INSERT INTO run_item (run_id, stage, item_key, input_hash, recipe_hash, state, batch, message)
VALUES (sqlc.arg(run_id), sqlc.arg(stage), sqlc.arg(item_key), sqlc.narg(input_hash), sqlc.narg(recipe_hash), 'pending', sqlc.arg(batch), sqlc.narg(message))
ON CONFLICT (run_id, stage, item_key) DO NOTHING;

-- name: ResetOlderFailedStageItems :exec
UPDATE run_item SET state = 'pending', updated_at = now()
WHERE run_id = sqlc.arg(run_id) AND stage = sqlc.arg(stage)
  AND state = 'failed' AND generation < sqlc.arg(generation);

-- name: ListPendingStageBatches :many
SELECT DISTINCT batch FROM run_item
WHERE run_id = $1 AND stage = $2 AND state = 'pending'
ORDER BY batch;
