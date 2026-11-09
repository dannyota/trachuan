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
