-- name: AllocateMailID :one
SELECT nextval(pg_get_serial_sequence('mail_outbox', 'id'))::bigint;

-- name: InsertSealedMail :one
INSERT INTO mail_outbox (id,kind,class,account_id,email_link_id,sealed_payload,key_id,expires_at)
OVERRIDING SYSTEM VALUE
VALUES ($1,$2,$3,$4,$5,$6,$7,now()+interval '24 hours') RETURNING id;

-- name: ClaimMailCandidate :one
UPDATE mail_outbox SET claimed_until=sqlc.arg(at)::timestamptz+interval '2 minutes', attempts=attempts+1
WHERE id=(SELECT id FROM mail_outbox WHERE sent_at IS NULL AND sealed_payload IS NOT NULL
 AND (next_attempt_at<=sqlc.arg(at)::timestamptz OR expires_at<=sqlc.arg(at)::timestamptz OR attempts>=6)
 AND (claimed_until IS NULL OR claimed_until<=sqlc.arg(at)::timestamptz)
 AND (expires_at<=sqlc.arg(at)::timestamptz OR attempts>=6 OR (
  (SELECT count(*) FROM mail_delivery d WHERE COALESCE(d.sent_at,d.reserved_at)>sqlc.arg(at)::timestamptz-interval '1 hour')<100
  AND (SELECT count(*) FROM mail_delivery d WHERE COALESCE(d.sent_at,d.reserved_at)>=date_trunc('month',sqlc.arg(at)::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')<2500
  AND (class<>'signup' OR (
   (SELECT count(*) FROM mail_delivery d WHERE d.class='signup' AND COALESCE(d.sent_at,d.reserved_at)>sqlc.arg(at)::timestamptz-interval '1 hour')<60
   AND (SELECT count(*) FROM mail_delivery d WHERE d.class='signup' AND COALESCE(d.sent_at,d.reserved_at)>=date_trunc('month',sqlc.arg(at)::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')<1500
  ))
 ))
 ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING *;

-- name: LockClaimedMail :one
SELECT * FROM mail_outbox WHERE id=$1 AND attempts=$2 AND sent_at IS NULL
 AND sealed_payload IS NOT NULL FOR UPDATE;

-- name: ResealMail :exec
UPDATE mail_outbox SET sealed_payload=$2,key_id=$3 WHERE id=$1;

-- name: PurgeMail :execrows
WITH expired AS (
 UPDATE mail_outbox SET sealed_payload=NULL WHERE sealed_payload IS NOT NULL AND expires_at<=now()
 AND LEAST(sent_at,expires_at)>=now()-interval '7 days'
)
DELETE FROM mail_outbox WHERE LEAST(sent_at,expires_at)<now()-interval '7 days';
