-- +goose Up
CREATE TABLE run_acquisition (
    run_id bigint NOT NULL REFERENCES run (id),
    stage text NOT NULL CHECK (stage IN ('expand', 'fetch')),
    document_key text NOT NULL,
    artifact_id bigint NOT NULL REFERENCES artifact (id),
    generation integer NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, stage, document_key)
);

-- +goose Down
DROP TABLE run_acquisition;
