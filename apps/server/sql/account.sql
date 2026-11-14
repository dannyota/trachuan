-- name: CreateAccount :one
INSERT INTO account (google_sub, email) VALUES (sqlc.arg(google_sub)::text, sqlc.arg(email)::text) RETURNING *;

-- name: GetAccount :one
SELECT * FROM account WHERE id = $1;

-- name: FindGoogleAccount :one
SELECT * FROM account WHERE google_sub = sqlc.arg(google_sub)::text;

-- name: AccountEmailExists :one
SELECT EXISTS (SELECT 1 FROM account WHERE email = $1);

-- name: RecordGoogleLogin :one
UPDATE account SET last_login_at = now() WHERE id = $1 RETURNING *;

-- name: InsertGoogleAccount :one
INSERT INTO account (google_sub, email, last_login_at) VALUES (sqlc.arg(google_sub)::text, sqlc.arg(email)::text, now())
ON CONFLICT DO NOTHING RETURNING *;

-- name: MarkAccountBlocked :one
UPDATE account SET blocked_at = COALESCE(blocked_at, now()) WHERE id = $1 RETURNING *;

-- name: MarkAccountDeleting :one
UPDATE account SET blocked_at = COALESCE(blocked_at, now()),
    delete_requested_at = COALESCE(delete_requested_at, now()) WHERE id = $1 RETURNING *;

-- name: BumpAccountEpoch :one
WITH bumped AS (
    UPDATE account SET auth_epoch = account.auth_epoch + 1 WHERE account.id = $1 RETURNING account.*
), sessions AS (
    UPDATE web_session SET revoked_at = now() WHERE account_id = $1 AND revoked_at IS NULL
), personal AS (
    UPDATE personal_token SET revoked_at = now() WHERE account_id = $1 AND revoked_at IS NULL
), grants AS (
    UPDATE oauth_grant SET revoked_at = COALESCE(revoked_at, now()) WHERE account_id = $1
), codes AS (
    UPDATE oauth_code SET revoked_at = COALESCE(revoked_at, now()) WHERE account_id = $1
), tokens AS (
    UPDATE oauth_token SET revoked_at = COALESCE(revoked_at, now()) WHERE account_id = $1
), pending AS (
    DELETE FROM pending_auth WHERE account_id = $1
), enrollments AS (
    DELETE FROM totp_enrollment WHERE account_id = $1
), links AS (
    UPDATE email_link SET consumed_at = now() WHERE account_id = $1 AND consumed_at IS NULL
)
SELECT * FROM bumped;

-- name: HasTOTP :one
SELECT EXISTS (SELECT 1 FROM totp_credential WHERE account_id = $1);

-- name: GetLiveAuthoritySession :one
SELECT web_session.*, statement_timestamp()::timestamptz AS checked_at FROM web_session WHERE id = $1 AND account_id = $2 AND auth_epoch = $3
    AND revoked_at IS NULL AND expires_at > statement_timestamp() AND created_at + interval '90 days' > statement_timestamp()
FOR SHARE;

-- name: PurgeDeletedAccounts :execrows
DELETE FROM account WHERE delete_requested_at < $1::timestamptz - interval '7 days';

-- name: LockCredentialAccount :one
SELECT * FROM account WHERE id = $1 FOR UPDATE;
