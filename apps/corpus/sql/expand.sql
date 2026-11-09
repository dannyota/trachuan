-- name: UpsertSeedObservation :exec
INSERT INTO run_seed_observation (run_id, seed_key, item_key, completeness, artifact_hash, observed_targets)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (run_id, seed_key, item_key) DO UPDATE SET completeness = EXCLUDED.completeness, artifact_hash = EXCLUDED.artifact_hash, observed_targets = EXCLUDED.observed_targets;

-- name: ListSeedObservations :many
SELECT * FROM run_seed_observation WHERE run_id = $1 ORDER BY seed_key COLLATE "C", item_key COLLATE "C";

-- name: ListStageItemArtifacts :many
SELECT run_item.item_key, run_item.state, artifact.path, artifact.sha256
FROM run_item LEFT JOIN artifact ON artifact.id = run_item.artifact_id
WHERE run_item.run_id = $1 AND run_item.stage = $2
ORDER BY run_item.item_key COLLATE "C";
