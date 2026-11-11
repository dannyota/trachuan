-- name: DeleteExpiredOAuthRequests :execrows
DELETE FROM oauth_request WHERE handle_hash IN (
    SELECT handle_hash FROM oauth_request WHERE expires_at <= sqlc.arg(as_of)::timestamptz AND sqlc.arg(retention)::interval IS NOT NULL ORDER BY expires_at LIMIT sqlc.arg(batch_limit)
);

-- name: DeleteExpiredOAuthCodes :execrows
DELETE FROM oauth_code WHERE code_hash IN (
    SELECT code_hash FROM oauth_code WHERE expires_at <= sqlc.arg(as_of)::timestamptz - sqlc.arg(retention)::interval
    ORDER BY expires_at LIMIT sqlc.arg(batch_limit)
);

-- name: DeleteExpiredOAuthTokens :execrows
WITH expired AS (
    SELECT token_hash FROM oauth_token
    WHERE family_expires_at <= sqlc.arg(as_of)::timestamptz - sqlc.arg(retention)::interval
    ORDER BY family_expires_at LIMIT sqlc.arg(batch_limit)
), revoked AS (
    SELECT t.token_hash FROM oauth_grant g
    CROSS JOIN LATERAL (
        SELECT token_hash FROM oauth_token WHERE grant_id = g.id OFFSET 0
    ) t
    WHERE g.revoked_at <= sqlc.arg(as_of)::timestamptz - sqlc.arg(retention)::interval
    ORDER BY g.revoked_at LIMIT sqlc.arg(batch_limit)
), candidates AS (
    SELECT token_hash FROM expired UNION SELECT token_hash FROM revoked
    LIMIT sqlc.arg(batch_limit)
)
DELETE FROM oauth_token WHERE token_hash IN (SELECT token_hash FROM candidates);

-- name: DeleteRetiredOAuthGrants :execrows
DELETE FROM oauth_grant AS target WHERE target.id IN (
    SELECT g.id FROM oauth_grant g
    WHERE g.retirement_anchor <= sqlc.arg(as_of)::timestamptz - sqlc.arg(retention)::interval
      AND NOT EXISTS (SELECT 1 FROM oauth_token t WHERE t.grant_id = g.id AND t.family_expires_at > sqlc.arg(as_of)::timestamptz)
      AND NOT EXISTS (SELECT 1 FROM oauth_code c WHERE c.grant_id = g.id AND c.expires_at > sqlc.arg(as_of)::timestamptz)
    ORDER BY g.retirement_anchor LIMIT sqlc.arg(batch_limit) FOR UPDATE SKIP LOCKED
) AND target.retirement_anchor <= sqlc.arg(as_of)::timestamptz - sqlc.arg(retention)::interval
  AND NOT EXISTS (SELECT 1 FROM oauth_token t WHERE t.grant_id = target.id AND t.family_expires_at > sqlc.arg(as_of)::timestamptz)
  AND NOT EXISTS (SELECT 1 FROM oauth_code c WHERE c.grant_id = target.id AND c.expires_at > sqlc.arg(as_of)::timestamptz);
