-- name: ListRunGapsByRun :many
SELECT * FROM gap WHERE run_id = $1 ORDER BY stage, item_key, kind, detail;

-- name: ResetRetryItem :execrows
UPDATE run_item SET state = 'pending', input_hash = NULL, artifact_id = NULL, attempt = 0,
    generation = sqlc.arg(generation), message = NULL, updated_at = now()
WHERE run_id = sqlc.arg(run_id) AND stage = sqlc.arg(stage) AND item_key = sqlc.arg(item_key);

-- name: DeleteItemGaps :execrows
DELETE FROM gap WHERE run_id = $1 AND stage = $2 AND item_key = $3;

-- name: DeleteStageItems :execrows
DELETE FROM run_item WHERE run_id = $1 AND stage = ANY(sqlc.arg(stages)::text[]);

-- name: DeleteStageGaps :execrows
DELETE FROM gap WHERE run_id = $1 AND stage = ANY(sqlc.arg(stages)::text[]);

-- name: DeleteStageMembershipEvidence :execrows
DELETE FROM run_membership_evidence
WHERE run_id = sqlc.arg(run_id)
  AND (stage = ANY(sqlc.arg(stages)::text[]) OR rule_kind = ANY(sqlc.arg(rule_kinds)::text[]));

-- name: DeleteStageSeedObservations :execrows
DELETE FROM run_seed_observation WHERE run_id = $1 AND stage = ANY(sqlc.arg(stages)::text[]);

-- name: DeleteUnsupportedRunDocuments :execrows
DELETE FROM run_document AS d
WHERE d.run_id = $1
  AND NOT EXISTS (SELECT 1 FROM run_membership_evidence AS e WHERE e.run_id = d.run_id AND e.document_key = d.document_key);

-- name: SetRunMessage :exec
UPDATE run SET message = $2, updated_at = now() WHERE id = $1;

-- name: DeleteRunListingsByQuery :execrows
DELETE FROM listing WHERE run_id = sqlc.arg(run_id) AND query_key = sqlc.arg(query_key);

-- name: DeleteRunListingsByItem :execrows
DELETE FROM listing
WHERE run_id = sqlc.arg(run_id) AND starts_with(query_key, sqlc.arg(item_key)::text || '@');
