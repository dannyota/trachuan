-- name: CreateSession :one
WITH revoked AS (
    UPDATE web_session SET revoked_at = now()
    WHERE revoked_at IS NULL AND (token_hash = ANY(COALESCE(sqlc.arg(replaced_hashes)::text[], ARRAY[]::text[])) OR id IN (
        SELECT s.id FROM web_session s WHERE s.account_id = sqlc.arg(account_id)
            AND s.revoked_at IS NULL AND s.token_hash <> ALL(COALESCE(sqlc.arg(replaced_hashes)::text[], ARRAY[]::text[]))
            AND s.expires_at > now() AND s.created_at + interval '90 days' > now()
        ORDER BY s.created_at DESC, s.id DESC OFFSET 49
    )) RETURNING token_hash
), created AS (
    INSERT INTO web_session (token_hash, account_id, csrf_secret, auth_epoch, expires_at, proof_at)
    SELECT sqlc.arg(token_hash), sqlc.arg(account_id), sqlc.arg(csrf_secret),
        sqlc.arg(auth_epoch), sqlc.arg(expires_at), now()
    WHERE EXISTS (SELECT 1 FROM account WHERE id = sqlc.arg(account_id) AND blocked_at IS NULL
        AND delete_requested_at IS NULL AND auth_epoch = sqlc.arg(auth_epoch))
    RETURNING *
)
SELECT created.*, ARRAY(SELECT token_hash FROM revoked)::text[] AS revoked_hashes FROM created;

-- name: GetSession :one
SELECT s.id, s.token_hash, s.account_id, s.csrf_secret,
    s.auth_epoch AS session_auth_epoch, s.created_at, s.last_seen_at, s.proof_at, s.factor_at,
    s.expires_at, s.revoked_at, a.blocked_at, a.delete_requested_at,
    a.auth_epoch AS account_auth_epoch
FROM web_session s JOIN account a ON a.id = s.account_id
WHERE s.token_hash = $1;

-- name: TouchSession :execrows
UPDATE web_session SET last_seen_at = statement_timestamp(),
    expires_at = LEAST(statement_timestamp() + interval '30 days', created_at + interval '90 days')
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > statement_timestamp()
    AND created_at + interval '90 days' > statement_timestamp()
    AND EXISTS (SELECT 1 FROM account a WHERE a.id = web_session.account_id
        AND a.blocked_at IS NULL AND a.delete_requested_at IS NULL AND a.auth_epoch = web_session.auth_epoch)
    AND last_seen_at <= statement_timestamp() - interval '1 hour';

-- name: RevokeSession :execrows
UPDATE web_session SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: ListSessions :many
SELECT * FROM web_session WHERE account_id = $1
    AND LEAST(expires_at, created_at + interval '90 days', revoked_at) >= now() - interval '7 days'
ORDER BY created_at DESC, id DESC LIMIT 100;

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
