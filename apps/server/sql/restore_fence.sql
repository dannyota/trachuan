-- name: ApplyRestoreFence :exec
WITH accounts AS (
    UPDATE account SET auth_epoch = auth_epoch + 1
), sessions AS (
    UPDATE web_session SET revoked_at = now() WHERE revoked_at IS NULL
), personal AS (
    UPDATE personal_token SET revoked_at = now() WHERE revoked_at IS NULL
), grants AS (
    UPDATE oauth_grant SET revoked_at = now() WHERE revoked_at IS NULL
), codes AS (
    UPDATE oauth_code SET revoked_at = now() WHERE revoked_at IS NULL
), tokens AS (
    UPDATE oauth_token SET revoked_at = now() WHERE revoked_at IS NULL
), pending AS (
    DELETE FROM pending_auth
), enrollments AS (
    DELETE FROM totp_enrollment
), links AS (
    UPDATE email_link SET consumed_at = now() WHERE consumed_at IS NULL
), requests AS (
    DELETE FROM oauth_request
), logins AS (
    DELETE FROM login_transaction
), entitlements AS (
    UPDATE framework_entitlement SET revoked_at = now(), revoked_by = 'restore-fence', revoke_reason = 'restore'
    WHERE revoked_at IS NULL RETURNING *
)
INSERT INTO framework_entitlement_event (entitlement_id, occurred_at, actor, action, email, scope, reason, note)
SELECT id, revoked_at, revoked_by, 'revoke', email, scope, revoke_reason, note FROM entitlements;
