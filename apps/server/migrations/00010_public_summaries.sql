-- +goose Up
ALTER TABLE corpus_release
    ADD COLUMN summary_database text,
    ADD COLUMN summary_projection integer NOT NULL DEFAULT 0 CHECK (summary_projection IN (0, 1)),
    ADD COLUMN headings_approved boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT corpus_release_heading_capability CHECK (NOT headings_approved OR summary_projection = 1);

-- +goose Down
ALTER TABLE corpus_release
    DROP CONSTRAINT corpus_release_heading_capability,
    DROP COLUMN summary_database,
    DROP COLUMN headings_approved,
    DROP COLUMN summary_projection;
