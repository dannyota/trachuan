-- name: GetPasswordByEmail :one
SELECT a.id AS account_id,a.email,a.auth_epoch,p.hash,p.version
FROM account a LEFT JOIN password_credential p ON p.account_id=a.id WHERE a.email=$1;

-- name: GetPasswordCredential :one
SELECT * FROM password_credential WHERE account_id=$1;

-- name: RecordPasswordProof :execrows
UPDATE web_session SET proof_at=now() WHERE id=$1 AND account_id=$2 AND auth_epoch=$3
 AND revoked_at IS NULL AND expires_at>now() AND created_at+interval '90 days'>now();
