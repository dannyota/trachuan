-- name: CreatePersonalToken :one
INSERT INTO personal_token (token_hash, account_id, name, expires_at, auth_epoch)
SELECT $1, $2, $3, $4, $5
WHERE EXISTS (SELECT 1 FROM account WHERE id = $2 AND blocked_at IS NULL
    AND delete_requested_at IS NULL AND auth_epoch = $5)
    AND (SELECT count(*) FROM personal_token WHERE account_id = $2
        AND revoked_at IS NULL AND expires_at > now()) < 20
RETURNING *;

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

-- name: ListTokens :many
SELECT * FROM personal_token WHERE account_id = $1
    AND LEAST(expires_at, revoked_at) >= now() - interval '7 days'
ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: RevokeOwnToken :one
UPDATE personal_token SET revoked_at = now()
WHERE account_id = $1 AND id = $2 AND revoked_at IS NULL
RETURNING token_hash;
