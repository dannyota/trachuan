-- name: UpsertListing :exec
INSERT INTO listing (run_id, query_key, item_key, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (run_id, query_key, item_key) DO UPDATE SET payload = EXCLUDED.payload;

-- name: InsertDocument :exec
INSERT INTO document (artifact_id, key, vbpl_id, number, title, doc_type, issuer, issue_date, eff_from, eff_to, status_code, status_name, is_consolidated, has_content)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: InsertFetchedFile :exec
INSERT INTO fetched_file (artifact_id, name, sha256, size, path)
VALUES ($1, $2, $3, $4, $5);
