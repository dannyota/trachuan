-- name: CreateSession :one
WITH revoked AS (
    UPDATE web_session SET revoked_at = now()
    WHERE id IN (
        SELECT s.id FROM web_session s WHERE s.account_id = $2 AND s.revoked_at IS NULL
            AND s.expires_at > now() AND s.created_at + interval '90 days' > now()
        ORDER BY s.created_at DESC, s.id DESC OFFSET 49
    ) RETURNING token_hash
), created AS (
    INSERT INTO web_session (token_hash, account_id, csrf_secret, auth_epoch, expires_at)
    SELECT $1, $2, $3, $4, $5
    WHERE EXISTS (SELECT 1 FROM account WHERE id = $2 AND blocked_at IS NULL
        AND delete_requested_at IS NULL AND auth_epoch = $4)
    RETURNING *
)
SELECT created.*, ARRAY(SELECT token_hash FROM revoked)::text[] AS revoked_hashes FROM created;

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
SELECT * FROM web_session WHERE account_id = $1
    AND LEAST(expires_at, created_at + interval '90 days', revoked_at) >= now() - interval '7 days'
ORDER BY created_at DESC, id DESC LIMIT 100;

-- Keep revocation, account state, auth epoch and both expiry bounds aligned
-- with validSession in apps/server/internal/websession/middleware.go.
-- name: ListLiveSessions :many
SELECT s.* FROM web_session s JOIN account a ON a.id = s.account_id
WHERE s.account_id = $1 AND s.revoked_at IS NULL
    AND a.blocked_at IS NULL AND a.delete_requested_at IS NULL
    AND s.auth_epoch = a.auth_epoch
    AND s.expires_at > now() AND s.created_at + interval '90 days' > now()
ORDER BY s.created_at DESC, s.id DESC LIMIT 100;

-- name: RevokeOwnSession :execrows
UPDATE web_session SET revoked_at = now()
WHERE account_id = $1 AND id = $2 AND revoked_at IS NULL;
