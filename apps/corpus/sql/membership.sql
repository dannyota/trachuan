-- name: UpsertRunDocument :exec
INSERT INTO run_document (run_id, document_key, country, source, source_id, class, disposition, raw_number, normalized_number, replacement_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (run_id, document_key) DO UPDATE SET country = EXCLUDED.country, source = EXCLUDED.source, source_id = EXCLUDED.source_id, class = EXCLUDED.class, disposition = EXCLUDED.disposition, raw_number = EXCLUDED.raw_number, normalized_number = EXCLUDED.normalized_number, replacement_key = EXCLUDED.replacement_key;

-- name: InsertMembershipEvidence :exec
INSERT INTO run_membership_evidence (run_id, document_key, stage, item_key, rule_kind, rule_value, query_identity, page, response_sha256, artifact_hash, evidence_path, support_state)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (run_id, document_key, stage, item_key, rule_kind, rule_value, query_identity, page, response_sha256, artifact_hash, evidence_path) DO UPDATE SET support_state = EXCLUDED.support_state;

-- name: ListRunDocuments :many
SELECT * FROM run_document WHERE run_id = $1 ORDER BY document_key COLLATE "C";

-- name: ListMembershipEvidence :many
SELECT * FROM run_membership_evidence WHERE run_id = $1 ORDER BY document_key COLLATE "C";

-- name: ListRunListings :many
SELECT query_key, item_key, payload FROM listing
WHERE run_id = $1 ORDER BY query_key COLLATE "C", item_key COLLATE "C";

-- name: GetRunItemArtifact :one
SELECT run_item.state, artifact.path, artifact.sha256
FROM run_item JOIN artifact ON artifact.id = run_item.artifact_id
WHERE run_item.run_id = $1 AND run_item.stage = $2 AND run_item.item_key = $3;

-- name: ListEarlierRuns :many
SELECT id, rel, snapshot FROM run WHERE country = $1 AND id <> $2;

-- name: ListLegacyExpectedKeys :many
SELECT run_item.item_key FROM run_item
WHERE run_item.run_id = $1 AND run_item.stage = 'fetch'
  AND NOT EXISTS (
    SELECT 1 FROM gap WHERE gap.run_id = run_item.run_id AND gap.stage = 'fetch'
      AND gap.item_key = run_item.item_key AND gap.kind IN ('scope_removed', 'withdrawn'))
ORDER BY run_item.item_key COLLATE "C";

-- name: ListLegacyListingSupports :many
SELECT listing.query_key, listing.item_key, artifact.sha256 AS artifact_sha256
FROM listing
JOIN run_item ON run_item.run_id = listing.run_id AND run_item.stage = 'discover'
  AND listing.query_key LIKE run_item.item_key || '@%'
JOIN artifact ON artifact.id = run_item.artifact_id
WHERE listing.run_id = $1
ORDER BY listing.item_key COLLATE "C", listing.query_key COLLATE "C";
