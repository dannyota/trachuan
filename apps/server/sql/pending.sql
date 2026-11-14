-- name: GetPendingAuth :one
SELECT p.* FROM pending_auth p JOIN account a ON a.id=p.account_id
WHERE p.token_hash=$1 AND p.consumed_at IS NULL AND p.expires_at>statement_timestamp()
 AND p.auth_epoch=a.auth_epoch AND a.blocked_at IS NULL AND a.delete_requested_at IS NULL
 AND EXISTS(SELECT 1 FROM totp_credential t WHERE t.account_id=a.id);

-- name: LockPendingAuth :one
SELECT p.* FROM pending_auth p JOIN account a ON a.id=p.account_id
WHERE p.token_hash=$1 AND p.consumed_at IS NULL AND p.expires_at>statement_timestamp()
 AND p.auth_epoch=a.auth_epoch AND a.blocked_at IS NULL AND a.delete_requested_at IS NULL
 AND EXISTS(SELECT 1 FROM totp_credential t WHERE t.account_id=a.id)
FOR UPDATE OF p;
