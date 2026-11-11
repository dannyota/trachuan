-- +goose Up
CREATE TABLE account (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    google_sub text NOT NULL UNIQUE,
    email text NOT NULL,
    blocked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz,
    delete_requested_at timestamptz,
    auth_epoch integer NOT NULL DEFAULT 0
);

CREATE TABLE personal_token (
    token_hash text PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    account_id bigint NOT NULL REFERENCES account(id),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz,
    auth_epoch integer NOT NULL,
    CHECK (expires_at <= created_at + interval '1 year')
);
CREATE INDEX personal_token_account_id_idx ON personal_token (account_id);

CREATE TABLE corpus_release (
    country text PRIMARY KEY,
    active_db text NOT NULL,
    activated_at timestamptz NOT NULL,
    activated_by text NOT NULL
);

CREATE TABLE retired_release (
    country text NOT NULL,
    db_name text NOT NULL,
    retired_at timestamptz NOT NULL,
    retain_until timestamptz NOT NULL,
    dropped_at timestamptz,
    PRIMARY KEY (country, db_name)
);

CREATE TABLE release_journal (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    country text NOT NULL,
    rel text NOT NULL,
    step text NOT NULL,
    state text NOT NULL,
    at timestamptz NOT NULL DEFAULT now(),
    message text NOT NULL
);

-- +goose Down
DROP TABLE release_journal;
DROP TABLE retired_release;
DROP TABLE corpus_release;
DROP TABLE personal_token;
DROP TABLE account;
