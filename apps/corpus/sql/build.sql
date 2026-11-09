-- name: GetBuildInventory :one
SELECT jsonb_build_object(
  'items', COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'stage', i.stage, 'item_key', i.item_key, 'state', i.state,
    'input_hash', i.input_hash, 'recipe_hash', i.recipe_hash,
    'artifact_sha256', a.sha256, 'path', a.path))
    FROM run_item i LEFT JOIN artifact a ON a.id = i.artifact_id
    WHERE i.run_id = sqlc.arg(run_id)), '[]'::jsonb),
  'gaps', COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'stage', g.stage, 'item_key', g.item_key, 'kind', g.kind, 'detail', g.detail))
    FROM gap g WHERE g.run_id = sqlc.arg(run_id)), '[]'::jsonb)
)::jsonb AS inventory;
