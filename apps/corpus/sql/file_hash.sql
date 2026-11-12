-- name: GetFileHash :one
SELECT * FROM file_hash WHERE sha256 = $1;

-- name: UpsertFileHash :exec
INSERT INTO file_hash (sha256, md5, etag_16, etag_5)
VALUES ($1, $2, $3, $4)
ON CONFLICT (sha256) DO UPDATE
SET md5 = EXCLUDED.md5, etag_16 = EXCLUDED.etag_16, etag_5 = EXCLUDED.etag_5;
