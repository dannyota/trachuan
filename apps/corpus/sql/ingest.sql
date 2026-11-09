-- name: UpsertListing :exec
INSERT INTO listing (run_id, query_key, item_key, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (run_id, query_key, item_key) DO UPDATE SET payload = EXCLUDED.payload;

-- name: InsertDocument :exec
INSERT INTO document (artifact_id, key, country, source, source_id, vbpl_id, number, title, doc_type, issuer, issue_date, eff_from, eff_to, status_code, status_name, is_consolidated, has_content)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);

-- name: InsertFetchedFile :exec
INSERT INTO fetched_file (artifact_id, name, sha256, size, path)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertProvision :exec
INSERT INTO provision (artifact_id, node_id, parent_node_id, kind, label, heading, ordinal, path, text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertRelation :exec
INSERT INTO relation (artifact_id, ordinal, kind, raw_code, raw_label, method, evidence, target_vbpl_id, target_number, source_node_id, target_node_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
