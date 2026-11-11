-- name: GetActiveRelease :one
SELECT * FROM corpus_release WHERE country = $1;

-- name: ListRetainedReleases :many
SELECT * FROM retired_release
WHERE country = $1 AND dropped_at IS NULL AND retain_until > now()
ORDER BY db_name;
