-- +goose Up
CREATE TABLE file_hash (
    sha256 text PRIMARY KEY,
    md5 text NOT NULL,
    etag_16 text,
    etag_5 text
);

-- +goose Down
DROP TABLE file_hash;
