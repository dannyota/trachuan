-- +goose Up
CREATE TABLE fetch_baseline (
    document_key text NOT NULL,
    fetch_recipe_hash text NOT NULL,
    artifact_id bigint NOT NULL REFERENCES artifact (id),
    run_id bigint NOT NULL REFERENCES run (id),
    item_key text NOT NULL,
    generation integer NOT NULL,
    attempt integer NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_key, fetch_recipe_hash)
);

-- +goose Down
DROP TABLE fetch_baseline;
