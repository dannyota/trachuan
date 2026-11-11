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
DROP TABLE ocr_check;
DROP TABLE ocr_text;
DROP FUNCTION ocr_immutable();
