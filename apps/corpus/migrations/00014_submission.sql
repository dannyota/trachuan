-- +goose Up
CREATE TABLE submission (
    id text COLLATE "C" PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
    run_id bigint NOT NULL REFERENCES run(id),
    stage text NOT NULL CHECK (stage = 'classify'),
    provider text NOT NULL CHECK (provider = 'anthropic'),
    screen_hash text COLLATE "C" NOT NULL CHECK (screen_hash ~ '^[0-9a-f]{64}$'),
    recipe jsonb NOT NULL,
    families jsonb NOT NULL,
    item_list jsonb NOT NULL CHECK (jsonb_typeof(item_list) = 'array'),
    item_list_hash text COLLATE "C" NOT NULL CHECK (item_list_hash ~ '^[0-9a-f]{64}$'),
    provider_job_id text,
    state text NOT NULL CHECK (state IN ('preparing','submitted','uncertain','abandoned','completed','cancelling','cancelled','failed')),
    created_at timestamptz NOT NULL,
    submitted_at timestamptz,
    deadline_at timestamptz,
    budget text NOT NULL CHECK (budget IN ('pilot','full','cap')),
    budget_micro_usd bigint NOT NULL CHECK (budget_micro_usd IN (15000000,100000000,150000000)),
    reserved_micro_usd bigint NOT NULL CHECK (reserved_micro_usd >= 0),
    cost_micro_usd bigint NOT NULL DEFAULT 0 CHECK (cost_micro_usd >= 0),
    cost_settled boolean NOT NULL DEFAULT false,
    results jsonb NOT NULL DEFAULT '{}',
    CHECK ((submitted_at IS NULL) = (deadline_at IS NULL)),
    CHECK (deadline_at IS NULL OR deadline_at = submitted_at + interval '26 hours'),
    CHECK (state NOT IN ('submitted','completed','cancelling','cancelled') OR provider_job_id IS NOT NULL),
    CHECK (state NOT IN ('completed','cancelled') OR cost_settled)
);
CREATE INDEX submission_run_stage ON submission (run_id,stage,created_at);
CREATE UNIQUE INDEX submission_active_screen ON submission (screen_hash) WHERE state IN ('preparing','submitted','uncertain','cancelling');

-- +goose Down
DROP TABLE submission;
