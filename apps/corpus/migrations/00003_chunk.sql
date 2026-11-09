-- +goose Up
CREATE TABLE chunk (
    artifact_id bigint NOT NULL REFERENCES artifact(id),
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    extract_artifact_id bigint NOT NULL,
    node_id text NOT NULL,
    anchor text NOT NULL,
    context_prefix text NOT NULL,
    content text NOT NULL,
    rough_tokens integer NOT NULL CHECK (rough_tokens > 0),
    spans jsonb NOT NULL CHECK (jsonb_typeof(spans) = 'array'),
    PRIMARY KEY (artifact_id, ordinal),
    FOREIGN KEY (extract_artifact_id, node_id) REFERENCES provision (artifact_id, node_id)
);

CREATE INDEX chunk_extract_node_idx ON chunk (extract_artifact_id, node_id);

-- +goose Down
DROP TABLE chunk;
