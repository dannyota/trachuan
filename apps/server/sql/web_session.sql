-- name: CreateSession :one
INSERT INTO web_session (token_hash, account_id, csrf_secret, auth_epoch, expires_at)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetSession :one
SELECT s.id, s.token_hash, s.account_id, s.csrf_secret,
    s.auth_epoch AS session_auth_epoch, s.created_at, s.last_seen_at,
    s.expires_at, s.revoked_at, a.blocked_at, a.delete_requested_at,
    a.auth_epoch AS account_auth_epoch
FROM web_session s JOIN account a ON a.id = s.account_id
WHERE s.token_hash = $1;

-- name: TouchSession :execrows
UPDATE web_session SET last_seen_at = now(),
    expires_at = LEAST(now() + interval '30 days', created_at + interval '90 days')
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
    AND last_seen_at <= now() - interval '1 hour';

-- name: RevokeSession :execrows
UPDATE web_session SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: ListSessions :many
SELECT * FROM web_session WHERE account_id = $1 ORDER BY id;

-- name: RevokeOwnSession :execrows
UPDATE web_session SET revoked_at = now()
WHERE account_id = $1 AND id = $2 AND revoked_at IS NULL;
