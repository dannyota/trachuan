-- +goose Up
CREATE TABLE frameworks_release (
    country text PRIMARY KEY,
    active_db text NOT NULL,
    activated_at timestamptz NOT NULL,
    activated_by text NOT NULL
);

REVOKE ALL ON frameworks_release FROM PUBLIC;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'server') THEN
        REVOKE ALL ON frameworks_release FROM server;
        GRANT SELECT ON frameworks_release TO server;
    END IF;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE frameworks_release;
