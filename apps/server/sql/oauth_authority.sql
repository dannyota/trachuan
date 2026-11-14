-- name: GetCodeIdentity :one
SELECT account_id, grant_id FROM oauth_code WHERE code_hash = $1;

-- name: LockGrantAuthority :one
SELECT account_id, client_id, resource, scope, revoked_at
FROM oauth_grant WHERE id = $1 FOR UPDATE;

-- name: PruneTokenFamilies :execrows
WITH excess AS (
    SELECT family_id FROM oauth_token
    WHERE grant_id = $1 AND revoked_at IS NULL AND family_expires_at > now()
    GROUP BY family_id ORDER BY min(created_at) DESC, family_id DESC OFFSET 9
)
UPDATE oauth_token SET revoked_at = COALESCE(revoked_at, now())
WHERE oauth_token.grant_id = $1 AND oauth_token.family_id IN (SELECT family_id FROM excess);
