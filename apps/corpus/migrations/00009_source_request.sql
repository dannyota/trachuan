-- +goose Up
CREATE TABLE run_source_request (
    run_id bigint NOT NULL REFERENCES run(id),
    stage text NOT NULL CHECK (stage IN ('discover', 'expand')),
    source text NOT NULL CHECK (source IN ('vbpl', 'chinhphu')),
    endpoint text NOT NULL CHECK (endpoint IN ('list', 'detail', 'diagram', 'home', 'search')),
    attempt integer NOT NULL CHECK (attempt > 0),
    started_at timestamptz NOT NULL,
    duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
    status integer,
    outcome text NOT NULL CHECK (outcome IN ('ok', 'http_error', 'invalid_body', 'timeout', 'transport', 'cancelled', 'paused'))
);

-- +goose Down
DROP TABLE run_source_request;
