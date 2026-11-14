-- name: PurgeEndedCredentials :execrows
WITH links AS (
 DELETE FROM email_link WHERE LEAST(consumed_at,expires_at)<statement_timestamp()-interval '7 days'
), pending AS (
 DELETE FROM pending_auth WHERE LEAST(consumed_at,expires_at)<statement_timestamp()-interval '7 days'
), enrollments AS (
 DELETE FROM totp_enrollment WHERE expires_at<statement_timestamp()-interval '7 days'
), recovery_attempts AS (
 DELETE FROM recovery_attempt WHERE at<=statement_timestamp()-interval '1 day'
), changes AS (
 DELETE FROM account_security_change WHERE created_at<statement_timestamp()-interval '7 days'
), tokens AS (
    DELETE FROM personal_token
    WHERE LEAST(expires_at, revoked_at) < now() - interval '7 days'
)
DELETE FROM web_session
WHERE LEAST(expires_at, created_at + interval '90 days', revoked_at) < now() - interval '7 days';
