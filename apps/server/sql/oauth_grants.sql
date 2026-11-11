-- name: UpsertGrant :one
INSERT INTO oauth_grant (account_id, client_id, scope, resource)
VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id, client_id) WHERE revoked_at IS NULL
DO UPDATE SET scope = EXCLUDED.scope, resource = EXCLUDED.resource, last_used_at = now()
RETURNING *;

-- name: CountLiveGrants :one
SELECT count(*) FROM oauth_grant WHERE account_id = $1 AND revoked_at IS NULL;

-- name: RevokeGrant :one
WITH grant_row AS (
    UPDATE oauth_grant SET revoked_at = COALESCE(revoked_at, now()) WHERE id = $1 RETURNING *
), codes AS (
    UPDATE oauth_code SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM grant_row) AND consumed_at IS NULL RETURNING code_hash
), tokens AS (
    UPDATE oauth_token SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM grant_row) RETURNING token_hash
)
SELECT * FROM grant_row;

-- name: ListGrants :many
SELECT g.*, c.name, c.kind FROM oauth_grant g JOIN oauth_client c ON c.client_id = g.client_id
WHERE g.account_id = $1 AND g.revoked_at IS NULL ORDER BY g.created_at DESC, g.id DESC LIMIT 100;

-- name: RevokeOwnGrant :one
WITH grant_row AS (
    UPDATE oauth_grant SET revoked_at = now() WHERE oauth_grant.account_id = $1 AND oauth_grant.id = $2 AND oauth_grant.revoked_at IS NULL RETURNING *
), codes AS (
    UPDATE oauth_code SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM grant_row) AND consumed_at IS NULL RETURNING code_hash
), tokens AS (
    UPDATE oauth_token SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM grant_row) RETURNING token_hash
)
SELECT * FROM grant_row;

-- name: TouchOAuthGrant :exec
UPDATE oauth_grant SET last_used_at = now()
WHERE id = $1 AND revoked_at IS NULL AND last_used_at <= now() - interval '1 hour';
