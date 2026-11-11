-- name: CreateRequest :one
INSERT INTO oauth_request (handle_hash, browser_hash, client_id, redirect_uri, state, code_challenge, resource, scope, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: ClaimRequest :one
UPDATE oauth_request SET claimed_at = now(), account_id = $3
WHERE handle_hash = $1 AND browser_hash = $2 AND claimed_at IS NULL AND expires_at > now()
RETURNING *;

-- name: CountPendingRequests :one
SELECT count(*) FROM oauth_request WHERE expires_at > now() AND claimed_at IS NULL;
