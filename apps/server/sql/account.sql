-- name: CreateAccount :one
INSERT INTO account (google_sub, email) VALUES ($1, $2) RETURNING *;

-- name: GetAccount :one
SELECT * FROM account WHERE id = $1;

-- name: BlockAccount :one
WITH blocked AS (
    UPDATE account SET blocked_at = COALESCE(blocked_at, now()),
        auth_epoch = auth_epoch + 1
    WHERE id = $1 RETURNING *
), revoked AS (
    UPDATE personal_token SET revoked_at = now()
    WHERE account_id IN (SELECT id FROM blocked) AND revoked_at IS NULL
)
SELECT * FROM blocked;
