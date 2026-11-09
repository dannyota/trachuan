-- name: UpsertListing :exec
INSERT INTO listing (run_id, query_key, item_key, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (run_id, query_key, item_key) DO UPDATE SET payload = EXCLUDED.payload;
