-- name: PurgeEndedCredentials :execrows
WITH tokens AS (
    DELETE FROM personal_token
    WHERE LEAST(expires_at, revoked_at) < now() - interval '7 days'
)
DELETE FROM web_session
WHERE LEAST(expires_at, created_at + interval '90 days', revoked_at) < now() - interval '7 days';
