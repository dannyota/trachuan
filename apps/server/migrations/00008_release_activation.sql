-- +goose Up
ALTER TABLE retired_release ADD COLUMN dump_sha256 text
    CHECK (dump_sha256 ~ '^[0-9a-f]{64}$');

ALTER TABLE release_journal
    ADD COLUMN operation_id text,
    ADD COLUMN mode text,
    ADD COLUMN database_name text,
    ADD COLUMN manifest_hash text,
    ADD COLUMN dump_sha256 text,
    ADD COLUMN expected_active_db text,
    ADD COLUMN manifest jsonb,
    ADD CONSTRAINT release_journal_operation CHECK (
        operation_id IS NULL OR (
            operation_id ~ '^[0-9a-f]{32}$'
            AND mode IS NOT NULL AND mode IN ('new', 'retained', 'prune')
            AND database_name IS NOT NULL
            AND database_name ~ '^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$'
            AND (mode = 'prune' OR (
                manifest_hash IS NOT NULL AND manifest_hash ~ '^[0-9a-f]{64}$'
                AND dump_sha256 IS NOT NULL AND dump_sha256 ~ '^[0-9a-f]{64}$'
                AND manifest IS NOT NULL AND jsonb_typeof(manifest) = 'object'
            ))
        )
    );
CREATE INDEX release_journal_operation_idx ON release_journal (operation_id, id DESC)
    WHERE operation_id IS NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'activator') THEN
        GRANT USAGE ON SCHEMA public TO activator;
        GRANT SELECT, INSERT, UPDATE, DELETE ON corpus_release, release_journal,
            retired_release TO activator;
        GRANT USAGE, SELECT ON SEQUENCE release_journal_id_seq TO activator;
    END IF;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'activator') THEN
        REVOKE SELECT, INSERT, UPDATE, DELETE ON corpus_release, release_journal,
            retired_release FROM activator;
        REVOKE USAGE, SELECT ON SEQUENCE release_journal_id_seq FROM activator;
    END IF;
END;
$$;
-- +goose StatementEnd
DROP INDEX release_journal_operation_idx;
ALTER TABLE release_journal
    DROP CONSTRAINT release_journal_operation,
    DROP COLUMN operation_id,
    DROP COLUMN mode,
    DROP COLUMN database_name,
    DROP COLUMN manifest_hash,
    DROP COLUMN dump_sha256,
    DROP COLUMN expected_active_db,
    DROP COLUMN manifest;
ALTER TABLE retired_release DROP COLUMN dump_sha256;
