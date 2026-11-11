-- name: CreateClient :one
INSERT INTO oauth_client (client_id, kind, name, redirect_uris, metadata_sha256, grant_types)
VALUES ($1, $2, $3, $4, $5, COALESCE(sqlc.narg(grant_types)::text[], ARRAY['authorization_code', 'refresh_token']::text[])) RETURNING *;

-- name: GetClient :one
SELECT * FROM oauth_client WHERE client_id = $1;

-- name: TouchClient :execrows
UPDATE oauth_client SET last_used_at = now()
WHERE client_id = $1 AND last_used_at <= now() - interval '1 hour';

-- name: CountClients :one
SELECT count(*) FROM oauth_client;

-- name: EvictClient :one
DELETE FROM oauth_client WHERE id = (
    SELECT c.id FROM oauth_client c
    WHERE NOT EXISTS (SELECT 1 FROM oauth_request r WHERE r.client_id = c.client_id AND r.expires_at > now() AND r.claimed_at IS NULL)
      AND NOT EXISTS (SELECT 1 FROM oauth_code x WHERE x.client_id = c.client_id AND x.expires_at > now() AND x.consumed_at IS NULL AND x.revoked_at IS NULL)
      AND NOT EXISTS (SELECT 1 FROM oauth_token t JOIN oauth_grant g ON g.id = t.grant_id WHERE t.client_id = c.client_id AND t.family_expires_at > now() AND t.revoked_at IS NULL AND g.revoked_at IS NULL)
    ORDER BY c.last_used_at, c.id LIMIT 1
) RETURNING *;
