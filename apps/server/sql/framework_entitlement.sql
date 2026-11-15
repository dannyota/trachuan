-- name: InsertFrameworkAllowance :one
WITH allowed AS (
    INSERT INTO framework_entitlement (email, scope, granted_by, note)
    VALUES ($1, $2, $3, $4) RETURNING *
), event AS (
    INSERT INTO framework_entitlement_event (entitlement_id, occurred_at, actor, action, email, scope, note)
    SELECT id, granted_at, granted_by, 'allow', email, scope, note FROM allowed
)
SELECT * FROM allowed;

-- name: ApplyFrameworkRevocation :one
WITH revoked AS (
    UPDATE framework_entitlement SET revoked_at = now(), revoked_by = sqlc.arg(actor), revoke_reason = 'operator'
    WHERE framework_entitlement.email = sqlc.arg(email) AND framework_entitlement.scope = sqlc.arg(scope)
        AND revoked_at IS NULL RETURNING *
), event AS (
    INSERT INTO framework_entitlement_event (entitlement_id, occurred_at, actor, action, email, scope, reason, note)
    SELECT id, revoked_at, revoked_by, 'revoke', email, scope, revoke_reason, note FROM revoked
)
SELECT * FROM revoked;

-- name: ListFrameworkEntitlements :many
SELECT * FROM framework_entitlement WHERE revoked_at IS NULL
    AND (sqlc.arg(email)::text = '' OR email = sqlc.arg(email)::text)
ORDER BY email, scope, id;

-- name: ListLiveFrameworkEntitlementsByEmail :many
SELECT * FROM framework_entitlement WHERE email = $1 AND revoked_at IS NULL
ORDER BY scope, id;
