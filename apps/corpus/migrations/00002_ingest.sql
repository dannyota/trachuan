-- +goose Up
CREATE TABLE listing (
    run_id bigint NOT NULL REFERENCES run(id),
    query_key text NOT NULL,
    item_key text NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY (run_id, query_key, item_key)
);

CREATE TABLE document (
    artifact_id bigint PRIMARY KEY REFERENCES artifact(id),
    key text NOT NULL,
    vbpl_id text NOT NULL,
    number text NOT NULL,
    title text NOT NULL,
    doc_type text NOT NULL,
    issuer text NOT NULL,
    issue_date text NOT NULL,
    eff_from text NOT NULL,
    eff_to text NOT NULL,
    status_code text NOT NULL,
    status_name text NOT NULL,
    is_consolidated boolean NOT NULL,
    has_content boolean NOT NULL
);

CREATE TABLE fetched_file (
    artifact_id bigint NOT NULL REFERENCES artifact(id),
    name text NOT NULL,
    sha256 text NOT NULL,
    size bigint NOT NULL,
    path text NOT NULL
);

CREATE TABLE provision (
    artifact_id bigint NOT NULL REFERENCES artifact(id),
    node_id text NOT NULL,
    parent_node_id text,
    kind text NOT NULL,
    label text NOT NULL,
    heading text NOT NULL,
    ordinal integer NOT NULL,
    path text NOT NULL,
    text text NOT NULL,
    UNIQUE (artifact_id, path),
    UNIQUE (artifact_id, node_id)
);

CREATE TABLE relation (
    artifact_id bigint NOT NULL REFERENCES artifact(id),
    ordinal integer NOT NULL,
    kind text NOT NULL,
    raw_code text NOT NULL,
    raw_label text NOT NULL,
    method text NOT NULL,
    evidence text NOT NULL,
    target_vbpl_id text NOT NULL,
    target_number text NOT NULL,
    source_node_id text,
    target_node_id text,
    PRIMARY KEY (artifact_id, ordinal)
);

-- +goose Down
DROP TABLE relation;
DROP TABLE provision;
DROP TABLE fetched_file;
DROP TABLE document;
DROP TABLE listing;
