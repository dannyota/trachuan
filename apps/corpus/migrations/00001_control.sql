-- +goose Up
CREATE TABLE run (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    country text NOT NULL CHECK (country ~ '^[a-z]{2}$'),
    rel text NOT NULL CHECK (rel ~ '^[0-9]{8}(_[0-9]{1,2})?$'),
    generation integer NOT NULL DEFAULT 0,
    state text NOT NULL CHECK (state IN ('starting', 'running', 'cancelling', 'cancelled', 'failed', 'paused', 'evaluated', 'released')),
    next_stage text,
    until_stage text,
    workflow_id text,
    snapshot jsonb NOT NULL,
    message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (country, rel)
);

CREATE TABLE artifact (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    stage text NOT NULL,
    item_key text NOT NULL,
    input_hash text NOT NULL,
    recipe_hash text NOT NULL,
    path text NOT NULL,
    sha256 text NOT NULL,
    written_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (stage, input_hash, recipe_hash)
);

CREATE TABLE run_item (
    run_id bigint NOT NULL REFERENCES run(id),
    stage text NOT NULL,
    item_key text NOT NULL,
    input_hash text,
    recipe_hash text,
    state text NOT NULL CHECK (state IN ('pending', 'done', 'gap', 'failed')),
    batch integer NOT NULL,
    generation integer NOT NULL DEFAULT 0,
    attempt integer NOT NULL DEFAULT 0,
    artifact_id bigint REFERENCES artifact(id),
    message text,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, stage, item_key)
);

CREATE TABLE gap (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id bigint NOT NULL REFERENCES run(id),
    stage text NOT NULL,
    item_key text NOT NULL,
    kind text NOT NULL,
    detail text NOT NULL,
    accepted_at timestamptz,
    note text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, stage, item_key, kind, detail)
);

-- +goose Down
DROP TABLE gap;
DROP TABLE run_item;
DROP TABLE artifact;
DROP TABLE run;
