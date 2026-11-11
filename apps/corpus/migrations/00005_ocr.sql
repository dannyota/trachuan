-- +goose Up
CREATE TABLE ocr_text (
    id bigserial PRIMARY KEY,
    pdf_sha256 text NOT NULL CHECK (pdf_sha256 ~ '^[0-9a-f]{64}$'),
    engine_id text NOT NULL CHECK (engine_id <> ''),
    text_sha256 text NOT NULL CHECK (text_sha256 ~ '^[0-9a-f]{64}$'),
    bytes bigint NOT NULL CHECK (bytes > 0),
    origin text NOT NULL CHECK (origin IN ('banhmi', 'vision')),
    pages integer CHECK (pages > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (pdf_sha256, engine_id)
);

CREATE TABLE ocr_check (
    id bigserial PRIMARY KEY,
    ocr_text_id bigint NOT NULL REFERENCES ocr_text (id),
    checker_id text NOT NULL CHECK (checker_id <> ''),
    result_sha256 text NOT NULL CHECK (result_sha256 ~ '^[0-9a-f]{64}$'),
    result jsonb NOT NULL CHECK (jsonb_typeof(result) = 'object'),
    flagged integer NOT NULL CHECK (flagged >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (ocr_text_id, checker_id)
);

CREATE TABLE ocr_check_attempt (
    id bigserial PRIMARY KEY,
    ocr_text_id bigint NOT NULL REFERENCES ocr_text (id),
    checker_id text NOT NULL CHECK (checker_id <> ''),
    mode text NOT NULL CHECK (mode IN ('direct', 'batch')),
    batch_id text,
    state text NOT NULL CHECK (state IN ('submitted', 'complete', 'failed')),
    reason text NOT NULL DEFAULT '',
    usage jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(usage) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((mode = 'batch') = (batch_id IS NOT NULL))
);
CREATE INDEX ON ocr_check_attempt (ocr_text_id, checker_id);
CREATE UNIQUE INDEX ON ocr_check_attempt (ocr_text_id, checker_id) WHERE state = 'submitted';

CREATE TABLE run_ocr_selection (
    run_id bigint NOT NULL REFERENCES run (id),
    item_key text NOT NULL,
    gap text NOT NULL DEFAULT '',
    pdf_sha256 text CHECK (pdf_sha256 ~ '^[0-9a-f]{64}$'),
    ocr_text_id bigint REFERENCES ocr_text (id),
    ocr_check_id bigint REFERENCES ocr_check (id),
    PRIMARY KEY (run_id, item_key),
    CHECK ((gap = '') = (ocr_text_id IS NOT NULL AND ocr_check_id IS NOT NULL AND pdf_sha256 IS NOT NULL)),
    CHECK (gap <> '' OR ocr_check_id IS NOT NULL)
);

-- +goose StatementBegin
CREATE FUNCTION ocr_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ocr store rows are immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER ocr_text_immutable BEFORE UPDATE OR DELETE ON ocr_text
    FOR EACH ROW EXECUTE FUNCTION ocr_immutable();
CREATE TRIGGER ocr_check_immutable BEFORE UPDATE OR DELETE ON ocr_check
    FOR EACH ROW EXECUTE FUNCTION ocr_immutable();

-- +goose Down
DROP TABLE run_ocr_selection;
DROP TABLE ocr_check_attempt;
DROP TABLE ocr_check;
DROP TABLE ocr_text;
DROP FUNCTION ocr_immutable();
