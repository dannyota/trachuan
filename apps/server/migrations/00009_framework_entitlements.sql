-- +goose Up
CREATE TABLE framework_entitlement (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email text NOT NULL CHECK (email = account_email(email)),
    scope text NOT NULL CHECK (scope = '*' OR scope COLLATE "C" ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    granted_at timestamptz NOT NULL DEFAULT now(),
    granted_by text NOT NULL CHECK (btrim(granted_by) <> ''),
    note text NOT NULL DEFAULT '',
    revoked_at timestamptz,
    revoked_by text,
    revoke_reason text,
    CHECK ((revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL)
        OR (revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND btrim(revoked_by) <> ''
            AND revoke_reason IS NOT NULL AND revoke_reason IN ('operator', 'restore')))
);
CREATE UNIQUE INDEX framework_entitlement_live_idx ON framework_entitlement (email, scope) WHERE revoked_at IS NULL;

CREATE TABLE framework_entitlement_event (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entitlement_id bigint NOT NULL REFERENCES framework_entitlement(id),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor text NOT NULL CHECK (btrim(actor) <> ''),
    action text NOT NULL CHECK (action IN ('allow', 'revoke')),
    email text NOT NULL CHECK (email = account_email(email)),
    scope text NOT NULL CHECK (scope = '*' OR scope COLLATE "C" ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    reason text,
    note text NOT NULL DEFAULT '',
    CHECK ((action = 'allow' AND reason IS NULL)
        OR (action = 'revoke' AND reason IS NOT NULL AND reason IN ('operator', 'restore')))
);
CREATE INDEX framework_entitlement_event_entitlement_idx ON framework_entitlement_event (entitlement_id, id);

-- +goose StatementBegin
CREATE FUNCTION reject_framework_entitlement_event_change() RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
    RAISE EXCEPTION 'framework entitlement events are immutable' USING ERRCODE = '55000';
END;
$fn$;
-- +goose StatementEnd
CREATE TRIGGER framework_entitlement_event_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON framework_entitlement_event
    FOR EACH STATEMENT EXECUTE FUNCTION reject_framework_entitlement_event_change();

-- +goose Down
DROP TABLE framework_entitlement_event;
DROP FUNCTION reject_framework_entitlement_event_change();
DROP TABLE framework_entitlement;
