-- +goose Up
CREATE TABLE embed_row (
    recipe_hash text NOT NULL,
    input_sha256 text NOT NULL,
    artifact_id bigint NOT NULL REFERENCES artifact(id),
    row integer NOT NULL,
    token_count integer NOT NULL,
    PRIMARY KEY (recipe_hash, input_sha256)
);

-- +goose Down
DROP TABLE embed_row;
