-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION account_email(raw text) RETURNS text LANGUAGE plpgsql IMMUTABLE STRICT AS $fn$
DECLARE
    address text;
    local_part text;
    domain_part text;
    label text;
BEGIN
    address := btrim(raw, E' \t\n\r\f\013' || chr(133) || chr(160) || chr(5760)
        || chr(8192) || chr(8193) || chr(8194) || chr(8195) || chr(8196)
        || chr(8197) || chr(8198) || chr(8199) || chr(8200) || chr(8201)
        || chr(8202) || chr(8232) || chr(8233) || chr(8239) || chr(8287) || chr(12288));
    local_part := split_part(address, '@', 1);
    domain_part := split_part(address, '@', 2);
    IF octet_length(address) > 254 OR octet_length(local_part) NOT BETWEEN 1 AND 64
        OR length(address) - length(replace(address, '@', '')) <> 1
        OR position('.' IN domain_part) = 0
        OR local_part COLLATE "C" !~ '^[a-zA-Z0-9!#$%&''*+/=?^_`{|}~-]+(\.[a-zA-Z0-9!#$%&''*+/=?^_`{|}~-]+)*$' THEN
        RAISE EXCEPTION 'invalid email address' USING ERRCODE = '23514';
    END IF;
    FOREACH label IN ARRAY string_to_array(domain_part, '.') LOOP
        IF octet_length(label) NOT BETWEEN 1 AND 63
            OR label COLLATE "C" !~ '^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$' THEN
            RAISE EXCEPTION 'invalid email address' USING ERRCODE = '23514';
        END IF;
    END LOOP;
    RETURN local_part || '@' || translate(domain_part, 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz');
END;
$fn$;
-- +goose StatementEnd

UPDATE account SET email = account_email(email);
ALTER TABLE account ALTER COLUMN google_sub DROP NOT NULL;
ALTER TABLE account ADD CONSTRAINT account_email_key UNIQUE (email);
ALTER TABLE web_session ADD COLUMN proof_at timestamptz, ADD COLUMN factor_at timestamptz;

CREATE TABLE password_credential (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    account_id bigint NOT NULL UNIQUE REFERENCES account(id) ON DELETE CASCADE,
    hash text NOT NULL,
    version bigint NOT NULL DEFAULT 1
);
CREATE TABLE email_link (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    purpose text NOT NULL CHECK (purpose IN ('registration', 'add_password', 'reset')),
    token_hash text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    email text NOT NULL,
    account_id bigint REFERENCES account(id) ON DELETE CASCADE,
    auth_epoch integer,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    CHECK ((purpose = 'registration' AND account_id IS NULL AND auth_epoch IS NULL)
        OR (purpose <> 'registration' AND account_id IS NOT NULL AND auth_epoch IS NOT NULL))
);
CREATE INDEX email_link_account_purpose_idx ON email_link (account_id, purpose);
CREATE INDEX email_link_email_purpose_idx ON email_link (email, purpose);
CREATE INDEX email_link_live_purpose_expiry_idx ON email_link (purpose, expires_at) WHERE consumed_at IS NULL;
CREATE INDEX email_link_expires_at_idx ON email_link (expires_at);
CREATE INDEX email_link_consumed_at_idx ON email_link (consumed_at) WHERE consumed_at IS NOT NULL;
CREATE TABLE pending_auth (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    token_hash text NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    csrf_secret text NOT NULL,
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    auth_epoch integer NOT NULL,
    primary_kind text NOT NULL CHECK (primary_kind IN ('google', 'password')),
    primary_at timestamptz NOT NULL,
    next text,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz
);
CREATE INDEX pending_auth_account_id_idx ON pending_auth (account_id);
CREATE INDEX pending_auth_expires_at_idx ON pending_auth (expires_at);
CREATE INDEX pending_auth_consumed_at_idx ON pending_auth (consumed_at) WHERE consumed_at IS NOT NULL;
CREATE TABLE totp_credential (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    account_id bigint NOT NULL UNIQUE REFERENCES account(id) ON DELETE CASCADE,
    sealed_secret bytea NOT NULL,
    key_id text NOT NULL,
    last_step bigint,
    failures integer NOT NULL DEFAULT 0,
    cooldown_until timestamptz,
    last_failure_at timestamptz
);
CREATE INDEX totp_credential_key_id_idx ON totp_credential (key_id);
CREATE TABLE totp_enrollment (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    account_id bigint NOT NULL UNIQUE REFERENCES account(id) ON DELETE CASCADE,
    web_session_id bigint NOT NULL REFERENCES web_session(id) ON DELETE CASCADE,
    sealed_secret bytea NOT NULL,
    key_id text NOT NULL,
    auth_epoch integer NOT NULL,
    failures integer NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL
);
CREATE INDEX totp_enrollment_web_session_id_idx ON totp_enrollment (web_session_id);
CREATE INDEX totp_enrollment_expires_at_idx ON totp_enrollment (expires_at);
CREATE INDEX totp_enrollment_key_id_idx ON totp_enrollment (key_id);
CREATE TABLE recovery_code (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    digest text NOT NULL UNIQUE CHECK (digest ~ '^[0-9a-f]{64}$'),
    used_at timestamptz
);
CREATE INDEX recovery_code_account_id_idx ON recovery_code (account_id);
CREATE TABLE recovery_attempt (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recovery_attempt_account_at_idx ON recovery_attempt (account_id, at);
CREATE TABLE mail_outbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    kind text NOT NULL CHECK (kind IN ('registration', 'account_exists', 'add_password', 'reset',
        'password_changed', 'totp_enrolled', 'totp_removed', 'recovery_codes_replaced')),
    class text NOT NULL CHECK (class IN ('signup', 'recovery', 'notice')),
    account_id bigint REFERENCES account(id) ON DELETE CASCADE,
    email_link_id bigint REFERENCES email_link(id) ON DELETE CASCADE,
    sealed_payload bytea,
    key_id text NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    claimed_until timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    sent_at timestamptz,
    CHECK ((account_id IS NULL) <> (email_link_id IS NULL))
);
CREATE INDEX mail_outbox_account_kind_created_idx ON mail_outbox (account_id, kind, created_at);
CREATE INDEX mail_outbox_email_link_id_idx ON mail_outbox (email_link_id);
CREATE INDEX mail_outbox_due_idx ON mail_outbox (next_attempt_at, claimed_until) WHERE sent_at IS NULL AND sealed_payload IS NOT NULL;
CREATE INDEX mail_outbox_expires_at_idx ON mail_outbox (expires_at);
CREATE INDEX mail_outbox_sent_at_idx ON mail_outbox (sent_at) WHERE sent_at IS NOT NULL;
CREATE INDEX mail_outbox_class_created_at_idx ON mail_outbox (class, created_at);
CREATE INDEX mail_outbox_class_sent_at_idx ON mail_outbox (class, sent_at) WHERE sent_at IS NOT NULL;
CREATE INDEX mail_outbox_key_id_idx ON mail_outbox (key_id) WHERE sealed_payload IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION require_account_credential() RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
    owner_id bigint;
BEGIN
    IF TG_TABLE_NAME = 'account' THEN
        owner_id := NEW.id;
    ELSE
        owner_id := OLD.account_id;
    END IF;
    PERFORM id FROM account WHERE id = owner_id FOR UPDATE;
    IF EXISTS (SELECT 1 FROM account WHERE id = owner_id AND google_sub IS NULL)
        AND NOT EXISTS (SELECT 1 FROM password_credential WHERE account_id = owner_id) THEN
        RAISE EXCEPTION 'account requires a primary credential' USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$fn$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER account_primary_credential
    AFTER INSERT OR UPDATE ON account DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION require_account_credential();
CREATE CONSTRAINT TRIGGER password_primary_credential
    AFTER DELETE OR UPDATE ON password_credential DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION require_account_credential();

-- +goose Down
ALTER TABLE account ALTER COLUMN google_sub SET NOT NULL;
DROP TRIGGER password_primary_credential ON password_credential;
DROP TRIGGER account_primary_credential ON account;
DROP FUNCTION require_account_credential();
DROP TABLE mail_outbox, recovery_attempt, recovery_code, totp_enrollment, totp_credential, pending_auth, email_link, password_credential;
ALTER TABLE web_session DROP COLUMN proof_at, DROP COLUMN factor_at;
ALTER TABLE account DROP CONSTRAINT account_email_key;
DROP FUNCTION account_email(text);
