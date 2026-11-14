-- +goose Up
ALTER TABLE login_transaction ADD COLUMN pending_hash text NOT NULL DEFAULT ''
    CHECK (pending_hash = '' OR pending_hash ~ '^[0-9a-f]{64}$');

-- +goose Down
ALTER TABLE login_transaction DROP COLUMN pending_hash;
