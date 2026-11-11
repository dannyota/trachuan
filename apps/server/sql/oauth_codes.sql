-- name: CreateCode :one
WITH grant_row AS (
    UPDATE oauth_grant SET retirement_anchor = GREATEST(retirement_anchor, sqlc.arg(expires_at)::timestamptz)
    WHERE id = sqlc.arg(grant_id) AND revoked_at IS NULL RETURNING *
)
INSERT INTO oauth_code (code_hash, grant_id, account_id, client_id, redirect_uri, resource, code_challenge, scope, auth_epoch, expires_at)
SELECT sqlc.arg(code_hash), id, account_id, client_id, sqlc.arg(redirect_uri), sqlc.arg(resource), sqlc.arg(code_challenge), sqlc.arg(scope), sqlc.arg(auth_epoch), sqlc.arg(expires_at)
FROM grant_row RETURNING *;

-- name: ConsumeCode :one
WITH locked AS (
    SELECT * FROM oauth_code WHERE oauth_code.code_hash = $1 AND revoked_at IS NULL
      AND (expires_at > now() OR consumed_at IS NOT NULL) FOR UPDATE
), updated AS (
    UPDATE oauth_code c SET consumed_at = COALESCE(c.consumed_at, now())
    FROM locked WHERE c.code_hash = locked.code_hash
    RETURNING c.*, (locked.consumed_at IS NOT NULL)::boolean AS replay
)
SELECT * FROM updated;
