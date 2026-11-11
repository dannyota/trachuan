-- name: CreateAccount :one
INSERT INTO account (google_sub, email) VALUES ($1, $2) RETURNING *;

-- name: GetAccount :one
SELECT * FROM account WHERE id = $1;

-- name: BlockAccount :one
WITH blocked AS (
    UPDATE account SET blocked_at = COALESCE(blocked_at, now()),
        auth_epoch = auth_epoch + 1
    WHERE account.id = $1 RETURNING *
), revoked_sessions AS (
    UPDATE web_session SET revoked_at = now()
    WHERE account_id IN (SELECT id FROM blocked) AND revoked_at IS NULL
), revoked AS (
    UPDATE personal_token SET revoked_at = now()
    WHERE account_id IN (SELECT id FROM blocked) AND revoked_at IS NULL
), revoked_grants AS (
    UPDATE oauth_grant SET revoked_at = COALESCE(revoked_at, now())
    WHERE account_id IN (SELECT id FROM blocked) RETURNING id
), revoked_codes AS (
    UPDATE oauth_code SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM revoked_grants)
), revoked_oauth_tokens AS (
    UPDATE oauth_token SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM revoked_grants)
)
SELECT * FROM blocked;

-- name: UpsertGoogleAccount :one
INSERT INTO account (google_sub, email, last_login_at) VALUES ($1, $2, now())
ON CONFLICT (google_sub) DO UPDATE SET email = EXCLUDED.email, last_login_at = now()
RETURNING *;

-- name: RequestDeletion :one
WITH blocked AS (
    UPDATE account SET blocked_at = COALESCE(blocked_at, now()),
        delete_requested_at = COALESCE(delete_requested_at, now()),
        auth_epoch = auth_epoch + 1
    WHERE account.id = $1 RETURNING *
), revoked_sessions AS (
    UPDATE web_session SET revoked_at = now()
    WHERE account_id IN (SELECT id FROM blocked) AND revoked_at IS NULL
), revoked_tokens AS (
    UPDATE personal_token SET revoked_at = now()
    WHERE account_id IN (SELECT id FROM blocked) AND revoked_at IS NULL
), revoked_grants AS (
    UPDATE oauth_grant SET revoked_at = COALESCE(revoked_at, now())
    WHERE account_id IN (SELECT id FROM blocked) RETURNING id
), revoked_codes AS (
    UPDATE oauth_code SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM revoked_grants)
), revoked_oauth_tokens AS (
    UPDATE oauth_token SET revoked_at = COALESCE(revoked_at, now())
    WHERE grant_id IN (SELECT id FROM revoked_grants)
)
SELECT * FROM blocked;

-- name: PurgeDeletedAccounts :execrows
DELETE FROM account WHERE delete_requested_at < $1::timestamptz - interval '7 days';

-- name: LockCredentialAccount :one
SELECT * FROM account WHERE id = $1 FOR UPDATE;
