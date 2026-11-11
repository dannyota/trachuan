-- +goose Up
ALTER TABLE personal_token ADD COLUMN id bigint NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE;
ALTER TABLE personal_token DROP CONSTRAINT personal_token_account_id_fkey;
ALTER TABLE personal_token ADD CONSTRAINT personal_token_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES account(id) ON DELETE CASCADE;

CREATE TABLE web_session (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    csrf_secret text NOT NULL,
    auth_epoch integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX web_session_account_id_idx ON web_session (account_id);

CREATE TABLE login_transaction (
    handle_hash text PRIMARY KEY CHECK (handle_hash ~ '^[0-9a-f]{64}$'),
    state text NOT NULL,
    nonce text NOT NULL,
    pkce_verifier text NOT NULL,
    next_path text NOT NULL,
    redirect_uri text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    claimed_at timestamptz
);
CREATE INDEX login_transaction_expires_at_idx ON login_transaction (expires_at);
CREATE INDEX login_transaction_claimed_at_idx ON login_transaction (claimed_at) WHERE claimed_at IS NOT NULL;
CREATE INDEX account_delete_requested_at_idx ON account (delete_requested_at) WHERE delete_requested_at IS NOT NULL;

-- +goose Down
DROP INDEX account_delete_requested_at_idx;
DROP TABLE login_transaction;
DROP TABLE web_session;
ALTER TABLE personal_token DROP CONSTRAINT personal_token_account_id_fkey;
ALTER TABLE personal_token ADD CONSTRAINT personal_token_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE personal_token DROP COLUMN id;
