-- +goose Up
CREATE TABLE account_security_change (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('reset', 'signed_in')),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
CREATE INDEX account_security_change_owner_at_idx ON account_security_change(account_id, kind, created_at);
CREATE TABLE mail_delivery (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    outbox_id bigint NOT NULL,
    attempt integer NOT NULL,
    class text NOT NULL CHECK (class IN ('signup', 'recovery', 'notice')),
    reserved_at timestamptz NOT NULL,
    sent_at timestamptz,
    UNIQUE(outbox_id, attempt)
);
CREATE INDEX mail_delivery_sent_idx ON mail_delivery(sent_at, class);

-- +goose Down
DROP TABLE mail_delivery, account_security_change;
