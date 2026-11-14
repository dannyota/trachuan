-- name: CreateLoginTransaction :one
INSERT INTO login_transaction (handle_hash, state, nonce, pkce_verifier, next_path, redirect_uri, expires_at, pending_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: ClaimLoginTransaction :one
UPDATE login_transaction SET claimed_at = now()
WHERE handle_hash = $1 AND claimed_at IS NULL AND expires_at > now()
RETURNING *;

-- name: DeleteExpiredLoginTransactions :exec
DELETE FROM login_transaction WHERE expires_at <= now() OR claimed_at IS NOT NULL;
