-- name: DeleteExpiredOAuthRequests :execrows
DELETE FROM oauth_request WHERE handle_hash IN (
    SELECT handle_hash FROM oauth_request WHERE expires_at <= sqlc.arg(as_of)::timestamptz AND sqlc.arg(retention)::interval IS NOT NULL ORDER BY expires_at LIMIT sqlc.arg(batch_limit)
);

-- name: DeleteExpiredOAuthCodes :execrows
DELETE FROM oauth_code WHERE code_hash IN (
    SELECT code_hash FROM oauth_code WHERE expires_at + sqlc.arg(retention)::interval <= sqlc.arg(as_of)::timestamptz
    ORDER BY expires_at LIMIT sqlc.arg(batch_limit)
);

-- name: DeleteExpiredOAuthTokens :execrows
DELETE FROM oauth_token WHERE token_hash IN (
    SELECT t.token_hash FROM oauth_token t JOIN oauth_grant g ON g.id = t.grant_id
    WHERE LEAST(t.family_expires_at, g.revoked_at) + sqlc.arg(retention)::interval <= sqlc.arg(as_of)::timestamptz
    ORDER BY LEAST(t.family_expires_at, g.revoked_at) LIMIT sqlc.arg(batch_limit)
);

-- name: DeleteRetiredOAuthGrants :execrows
DELETE FROM oauth_grant WHERE id IN (
    SELECT g.id FROM oauth_grant g
    WHERE g.retirement_anchor + sqlc.arg(retention)::interval <= sqlc.arg(as_of)::timestamptz
      AND NOT EXISTS (SELECT 1 FROM oauth_token t WHERE t.grant_id = g.id AND t.family_expires_at > sqlc.arg(as_of)::timestamptz)
      AND NOT EXISTS (SELECT 1 FROM oauth_code c WHERE c.grant_id = g.id AND c.expires_at > sqlc.arg(as_of)::timestamptz)
    ORDER BY g.retirement_anchor LIMIT sqlc.arg(batch_limit)
);
