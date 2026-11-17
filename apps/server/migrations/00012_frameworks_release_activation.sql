-- +goose Up
ALTER TABLE release_journal
    ADD COLUMN corpus_kind text NOT NULL DEFAULT 'law' CHECK (corpus_kind IN ('law', 'frameworks')),
    ADD CONSTRAINT release_journal_kind CHECK (
        (corpus_kind = 'law' AND database_name ~ '^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$'
            AND expected_active_db ~ '^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$') OR
        (corpus_kind = 'frameworks' AND database_name ~ '^frameworks_[0-9]{8}(_[0-9]+)?$'
            AND expected_active_db ~ '^frameworks_[0-9]{8}(_[0-9]+)?$')
    ) NOT VALID,
    DROP CONSTRAINT release_journal_operation,
    ADD CONSTRAINT release_journal_operation CHECK (
        operation_id IS NULL OR (
            operation_id ~ '^[0-9a-f]{32}$'
            AND mode IS NOT NULL AND mode IN ('new', 'retained', 'prune')
            AND database_name IS NOT NULL
            AND database_name ~ '^(corpus_[a-z]{2}|frameworks)_[0-9]{8}(_[0-9]+)?$'
            AND (mode = 'prune' OR (
                manifest_hash IS NOT NULL AND manifest_hash ~ '^[0-9a-f]{64}$'
                AND dump_sha256 IS NOT NULL AND dump_sha256 ~ '^[0-9a-f]{64}$'
                AND manifest IS NOT NULL AND jsonb_typeof(manifest) = 'object'
            ))
        )
    );
ALTER TABLE retired_release
    ADD COLUMN corpus_kind text NOT NULL DEFAULT 'law' CHECK (corpus_kind IN ('law', 'frameworks')),
    ADD CONSTRAINT retired_release_kind CHECK (
        (corpus_kind = 'law' AND db_name ~ '^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$') OR
        (corpus_kind = 'frameworks' AND db_name ~ '^frameworks_[0-9]{8}(_[0-9]+)?$')
    ) NOT VALID;
ALTER TABLE frameworks_release
    ADD CONSTRAINT frameworks_release_country CHECK (country = 'vn'),
    ADD CONSTRAINT frameworks_release_active_db CHECK (active_db ~ '^frameworks_[0-9]{8}(_[0-9]+)?$');

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'activator') THEN
        GRANT SELECT, INSERT, UPDATE ON frameworks_release TO activator;
    END IF;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'activator') THEN
        REVOKE SELECT, INSERT, UPDATE ON frameworks_release FROM activator;
    END IF;
END;
$$;
-- +goose StatementEnd
ALTER TABLE frameworks_release
    DROP CONSTRAINT frameworks_release_country,
    DROP CONSTRAINT frameworks_release_active_db;
ALTER TABLE retired_release
    DROP CONSTRAINT retired_release_kind,
    DROP COLUMN corpus_kind;
ALTER TABLE release_journal
    DROP CONSTRAINT release_journal_kind,
    DROP COLUMN corpus_kind,
    DROP CONSTRAINT release_journal_operation,
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
