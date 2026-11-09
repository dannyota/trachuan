-- +goose Up
ALTER TABLE run ADD CONSTRAINT run_id_country_unique UNIQUE (id, country);

CREATE TABLE run_document (
    run_id bigint NOT NULL,
    document_key text NOT NULL,
    country text NOT NULL,
    source text NOT NULL,
    source_id text NOT NULL,
    class text NOT NULL CHECK (class IN ('primary', 'relation_context', 'secondary')),
    disposition text NOT NULL CHECK (disposition IN ('active', 'withdrawn', 'scope_removed', 'source_replaced')),
    raw_number text,
    normalized_number text,
    replacement_key text,
    PRIMARY KEY (run_id, document_key),
    FOREIGN KEY (run_id, country) REFERENCES run (id, country),
    CHECK (document_key = country || ':' || source || ':' || source_id),
    CHECK ((disposition = 'source_replaced') = (replacement_key IS NOT NULL))
);

CREATE TABLE run_membership_evidence (
    run_id bigint NOT NULL,
    document_key text NOT NULL,
    stage text NOT NULL,
    item_key text NOT NULL,
    rule_kind text NOT NULL CHECK (rule_kind IN ('agency', 'term', 'named', 'secondary_probe', 'relation')),
    rule_value text NOT NULL,
    query_identity text,
    page text,
    response_sha256 text NOT NULL,
    artifact_hash text NOT NULL,
    evidence_path text NOT NULL,
    support_state text NOT NULL CHECK (support_state IN ('current', 'retained', 'removed')),
    FOREIGN KEY (run_id, document_key) REFERENCES run_document (run_id, document_key),
    FOREIGN KEY (run_id, stage, item_key) REFERENCES run_item (run_id, stage, item_key),
    UNIQUE NULLS NOT DISTINCT (run_id, document_key, stage, item_key, rule_kind, rule_value, query_identity, page, response_sha256, artifact_hash, evidence_path)
);

CREATE TABLE run_seed_observation (
    run_id bigint NOT NULL REFERENCES run(id),
    seed_key text NOT NULL,
    stage text NOT NULL DEFAULT 'expand' CHECK (stage = 'expand'),
    item_key text NOT NULL,
    completeness text NOT NULL CHECK (completeness IN ('complete', 'incomplete')),
    artifact_hash text NOT NULL,
    observed_targets jsonb NOT NULL CHECK (jsonb_typeof(observed_targets) = 'array'),
    PRIMARY KEY (run_id, seed_key, item_key),
    FOREIGN KEY (run_id, stage, item_key) REFERENCES run_item (run_id, stage, item_key)
);

CREATE TABLE run_scope (
    run_id bigint PRIMARY KEY REFERENCES run(id),
    seeds_snapshot bytea, seeds_hash text,
    expanded_snapshot bytea, expanded_hash text,
    final_snapshot bytea, final_hash text,
    fetch_close_input_hash text,
    CHECK ((seeds_snapshot IS NULL) = (seeds_hash IS NULL)),
    CHECK ((expanded_snapshot IS NULL) = (expanded_hash IS NULL)),
    CHECK ((final_snapshot IS NULL) = (final_hash IS NULL))
);

CREATE TABLE run_failure (
    run_id bigint NOT NULL REFERENCES run(id),
    failure_identity text NOT NULL,
    code text NOT NULL,
    stage text NOT NULL,
    item_key text NOT NULL,
    reason text NOT NULL,
    evidence_refs jsonb NOT NULL CHECK (jsonb_typeof(evidence_refs) = 'array'),
    PRIMARY KEY (run_id, failure_identity)
);

CREATE TABLE run_source (
    run_id bigint NOT NULL REFERENCES run(id),
    source text NOT NULL,
    state text NOT NULL CHECK (state IN ('active', 'source_paused')),
    pause_reason text,
    evidence_ref text,
    paused_at timestamptz,
    cleared_at timestamptz,
    PRIMARY KEY (run_id, source)
);

CREATE TABLE run_source_event (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id bigint NOT NULL,
    source text NOT NULL,
    event text NOT NULL CHECK (event IN ('pause', 'clear')),
    reason text,
    evidence_ref text,
    clear_note text,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (run_id, source) REFERENCES run_source (run_id, source)
);

ALTER TABLE gap
    ADD COLUMN source text,
    ADD COLUMN origin text CHECK (origin IN ('gateway', 'source_data', 'transport', 'unknown')),
    ADD COLUMN cause text CHECK (cause IN ('http_5xx', 'http_429', 'non_json_200', 'timeout', 'connection_error', 'invalid_payload', 'unknown')),
    ADD COLUMN endpoint text,
    ADD COLUMN http_status integer,
    ADD COLUMN evidence_ref text;

ALTER TABLE document
    ADD COLUMN country text,
    ADD COLUMN source text,
    ADD COLUMN source_id text,
    ADD COLUMN source_url text,
    ADD COLUMN identity_evidence jsonb;
UPDATE document SET country = split_part(key, ':', 1), source = 'vbpl', source_id = vbpl_id;
ALTER TABLE document
    ALTER COLUMN country SET NOT NULL,
    ALTER COLUMN source SET NOT NULL,
    ALTER COLUMN source_id SET NOT NULL,
    ALTER COLUMN vbpl_id DROP NOT NULL,
    ADD CONSTRAINT document_source_identity_check CHECK ((source = 'vbpl' AND vbpl_id IS NOT NULL AND vbpl_id = source_id) OR (source <> 'vbpl' AND vbpl_id IS NULL)),
    ADD CONSTRAINT document_key_check CHECK (key = country || ':' || source || ':' || source_id),
    ADD CONSTRAINT document_artifact_source_unique UNIQUE (artifact_id, source);

ALTER TABLE fetched_file
    ADD COLUMN source text NOT NULL DEFAULT 'vbpl',
    ADD COLUMN url text,
    ADD COLUMN role text NOT NULL DEFAULT 'unknown' CHECK (role IN ('main', 'appendix', 'scan', 'unknown')),
    ADD COLUMN fetched_at timestamptz,
    ADD CONSTRAINT fetched_file_document_source_fk FOREIGN KEY (artifact_id, source) REFERENCES document (artifact_id, source);

-- +goose Down
ALTER TABLE fetched_file DROP COLUMN fetched_at, DROP COLUMN role, DROP COLUMN url, DROP COLUMN source;
ALTER TABLE document DROP CONSTRAINT document_artifact_source_unique, DROP CONSTRAINT document_key_check, DROP CONSTRAINT document_source_identity_check;
ALTER TABLE document ALTER COLUMN vbpl_id SET NOT NULL;
ALTER TABLE document DROP COLUMN identity_evidence, DROP COLUMN source_url, DROP COLUMN source_id, DROP COLUMN source, DROP COLUMN country;
ALTER TABLE gap DROP COLUMN evidence_ref, DROP COLUMN http_status, DROP COLUMN endpoint, DROP COLUMN cause, DROP COLUMN origin, DROP COLUMN source;
DROP TABLE run_source_event;
DROP TABLE run_source;
DROP TABLE run_failure;
DROP TABLE run_scope;
DROP TABLE run_seed_observation;
DROP TABLE run_membership_evidence;
DROP TABLE run_document;
ALTER TABLE run DROP CONSTRAINT run_id_country_unique;
