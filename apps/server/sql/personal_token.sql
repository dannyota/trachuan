-- name: CreatePersonalToken :one
INSERT INTO personal_token (token_hash, account_id, name, expires_at, auth_epoch)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetCredential :one
SELECT a.id AS account_id, a.blocked_at, a.auth_epoch AS account_auth_epoch,
    t.auth_epoch AS token_auth_epoch, t.expires_at, t.revoked_at
FROM personal_token t JOIN account a ON a.id = t.account_id
WHERE t.token_hash = $1;

-- name: RevokePersonalToken :exec
UPDATE personal_token SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: TouchPersonalToken :exec
UPDATE personal_token SET last_used_at = now()
WHERE token_hash = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute');
