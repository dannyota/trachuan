-- name: UpsertRunAcquisition :exec
INSERT INTO run_acquisition (run_id, stage, document_key, artifact_id, generation)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id, stage, document_key)
DO UPDATE SET artifact_id = EXCLUDED.artifact_id, generation = EXCLUDED.generation, recorded_at = now();

-- name: GetRunAcquisition :one
SELECT a.run_id, a.stage, a.document_key, a.artifact_id, a.generation,
       f.stage AS artifact_stage, f.item_key AS artifact_item_key, f.input_hash, f.recipe_hash, f.path, f.sha256
FROM run_acquisition a JOIN artifact f ON f.id = a.artifact_id
WHERE a.run_id = $1 AND a.stage = $2 AND a.document_key = $3;

-- name: CountRunAcquisitions :one
SELECT count(*) FROM run_acquisition WHERE run_id = $1;

-- name: CountAdoptedRunItems :one
SELECT count(*) FROM run_item
WHERE run_id = $1 AND generation = $2
  AND stage IN ('expand', 'fetch') AND state = 'done'
  AND artifact_id IS NOT NULL AND message = $3;

-- name: SeedRunAcquisitions :execrows
-- Seeds acquisitions from the run's own committed done items; used once,
-- before the first retry invalidation of a run that has none.
INSERT INTO run_acquisition (run_id, stage, document_key, artifact_id, generation)
SELECT i.run_id, i.stage, i.item_key, i.artifact_id, i.generation
FROM run_item i
WHERE i.run_id = $1 AND i.stage IN ('expand', 'fetch') AND i.state = 'done' AND i.artifact_id IS NOT NULL
ON CONFLICT (run_id, stage, document_key) DO NOTHING;

-- name: CountFetchReads :one
SELECT
  count(*) FILTER (WHERE message = 'read=reused') AS reused,
  count(*) FILTER (WHERE message = 'read=no_baseline') AS no_baseline,
  count(*) FILTER (WHERE message = 'read=invalid_baseline') AS invalid_baseline,
  count(*) FILTER (WHERE message = 'read=null_date') AS null_date,
  count(*) FILTER (WHERE message = 'read=date_changed') AS date_changed,
  count(*) FILTER (WHERE message = 'read=missing_attachment') AS missing_attachment,
  count(*) FILTER (WHERE message = 'read=age') AS age,
  count(*) FILTER (WHERE message = 'read=control' OR message LIKE 'read=control outcome=%') AS control,
  count(*) FILTER (WHERE message = 'read=control outcome=same') AS control_same,
  count(*) FILTER (WHERE message = 'read=control outcome=tree_ids_only') AS control_tree_ids_only,
  count(*) FILTER (WHERE message = 'read=control outcome=changed') AS control_changed
FROM run_item
WHERE run_id = $1 AND generation = $2 AND stage = 'fetch' AND state = 'done'
  AND item_key LIKE 'vn:vbpl:%';
