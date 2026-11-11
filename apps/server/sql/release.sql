-- name: GetActiveRelease :one
SELECT * FROM corpus_release WHERE country = $1;
