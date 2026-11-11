-- name: CreateTokenFamily :one
WITH grant_row AS (
    UPDATE oauth_grant SET retirement_anchor = GREATEST(retirement_anchor, sqlc.arg(family_expires_at)::timestamptz)
    WHERE id = sqlc.arg(grant_id) AND revoked_at IS NULL RETURNING *
)
INSERT INTO oauth_token (token_hash, kind, grant_id, account_id, client_id, family_id, rotated_from, resource, scope, auth_epoch, expires_at, family_expires_at)
SELECT sqlc.arg(token_hash), sqlc.arg(kind), id, account_id, client_id, sqlc.arg(family_id), sqlc.narg(rotated_from), sqlc.arg(resource), sqlc.arg(scope), sqlc.arg(auth_epoch), sqlc.arg(expires_at), sqlc.arg(family_expires_at)
FROM grant_row RETURNING *;

-- name: RotateRefresh :one
WITH locked AS (
    SELECT * FROM oauth_token WHERE oauth_token.token_hash = $1 AND kind = 'refresh' AND revoked_at IS NULL
      AND family_expires_at > now() FOR UPDATE
), updated AS (
    UPDATE oauth_token t SET superseded_at = COALESCE(t.superseded_at, now())
    FROM locked WHERE t.token_hash = locked.token_hash
    RETURNING t.*, (locked.superseded_at IS NOT NULL)::boolean AS reuse
)
SELECT * FROM updated;

-- name: GetToken :one
SELECT t.*, g.revoked_at AS grant_revoked_at, a.blocked_at, a.delete_requested_at, a.auth_epoch AS account_auth_epoch
FROM oauth_token t JOIN oauth_grant g ON g.id = t.grant_id JOIN account a ON a.id = t.account_id
WHERE t.token_hash = $1;

-- name: RevokeToken :execrows
UPDATE oauth_token SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;
