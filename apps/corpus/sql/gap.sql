-- name: InsertGap :exec
INSERT INTO gap (run_id, stage, item_key, kind, detail)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id, stage, item_key, kind, detail) DO NOTHING;
