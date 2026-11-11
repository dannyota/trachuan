-- +goose Up
CREATE TABLE oauth_client (
    id bigint NOT NULL GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_id text NOT NULL UNIQUE,
    kind text NOT NULL CHECK (kind IN ('cimd', 'preregistered', 'dcr')),
    name text NOT NULL,
    redirect_uris text[] NOT NULL,
    metadata_sha256 text CHECK (metadata_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_request (
    handle_hash text PRIMARY KEY CHECK (handle_hash ~ '^[0-9a-f]{64}$'),
    browser_hash text NOT NULL CHECK (browser_hash ~ '^[0-9a-f]{64}$'),
    client_id text NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    state text NOT NULL,
    code_challenge text NOT NULL,
    resource text NOT NULL,
    scope text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    claimed_at timestamptz,
    account_id bigint REFERENCES account(id) ON DELETE CASCADE
);
CREATE INDEX oauth_request_expires_at_idx ON oauth_request (expires_at);
CREATE INDEX oauth_request_client_id_idx ON oauth_request (client_id);

CREATE TABLE oauth_grant (
    id bigint NOT NULL GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    client_id text NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    scope text NOT NULL,
    resource text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now(),
    retirement_anchor timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE UNIQUE INDEX oauth_grant_live_idx ON oauth_grant (account_id, client_id) WHERE revoked_at IS NULL;
CREATE INDEX oauth_grant_client_id_idx ON oauth_grant (client_id);
CREATE INDEX oauth_grant_retirement_anchor_idx ON oauth_grant (retirement_anchor);
CREATE INDEX oauth_grant_revoked_at_idx ON oauth_grant (revoked_at) WHERE revoked_at IS NOT NULL;

CREATE TABLE oauth_code (
    code_hash text PRIMARY KEY CHECK (code_hash ~ '^[0-9a-f]{64}$'),
    grant_id bigint NOT NULL REFERENCES oauth_grant(id) ON DELETE CASCADE,
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    client_id text NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    resource text NOT NULL,
    code_challenge text NOT NULL,
    scope text NOT NULL,
    auth_epoch integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz
);
CREATE INDEX oauth_code_grant_id_idx ON oauth_code (grant_id);
CREATE INDEX oauth_code_client_id_idx ON oauth_code (client_id);
CREATE INDEX oauth_code_expires_at_idx ON oauth_code (expires_at);

CREATE TABLE oauth_token (
    token_hash text PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    kind text NOT NULL CHECK (kind IN ('access', 'refresh')),
    grant_id bigint NOT NULL REFERENCES oauth_grant(id) ON DELETE CASCADE,
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    client_id text NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    family_id text NOT NULL CHECK (family_id ~ '^[0-9a-f]{64}$'),
    rotated_from text CHECK (rotated_from ~ '^[0-9a-f]{64}$'),
    resource text NOT NULL,
    scope text NOT NULL,
    auth_epoch integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    family_expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    superseded_at timestamptz,
    last_used_at timestamptz
);
CREATE INDEX oauth_token_grant_id_idx ON oauth_token (grant_id);
CREATE INDEX oauth_token_client_id_idx ON oauth_token (client_id);
CREATE INDEX oauth_token_family_expires_at_idx ON oauth_token (family_expires_at);

-- +goose Down
DROP TABLE oauth_token;
DROP TABLE oauth_code;
DROP TABLE oauth_grant;
DROP TABLE oauth_request;
DROP TABLE oauth_client;
