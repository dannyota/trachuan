-- +goose Up
CREATE TABLE issue_report (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    release text NOT NULL,
    document_key text NOT NULL,
    anchor text NOT NULL,
    document_number text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('wrong_text', 'wrong_validity', 'broken_citation', 'missing_document')),
    target text NOT NULL,
    suggestion jsonb NOT NULL CHECK (jsonb_typeof(suggestion) = 'object'),
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'accepted', 'rejected', 'withdrawn')),
    created_at timestamptz NOT NULL DEFAULT now(),
    changed_at timestamptz NOT NULL DEFAULT now(),
    decided_by text,
    export_batch text
);
CREATE UNIQUE INDEX issue_report_open_idx ON issue_report (account_id, document_key, anchor, kind, target) WHERE state = 'open';
CREATE INDEX issue_report_own_idx ON issue_report (account_id, created_at DESC, id DESC);
CREATE INDEX issue_report_triage_idx ON issue_report (document_key, anchor, kind, target, state);
CREATE INDEX issue_report_retention_idx ON issue_report (state, changed_at);
CREATE INDEX issue_report_daily_idx ON issue_report (account_id, created_at);

CREATE TABLE issue_report_export (
    batch_id text NOT NULL,
    report_id bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    document_key text NOT NULL,
    anchor text NOT NULL,
    document_number text NOT NULL,
    release text NOT NULL,
    kind text NOT NULL,
    target text NOT NULL,
    suggestion jsonb NOT NULL CHECK (jsonb_typeof(suggestion) = 'object'),
    PRIMARY KEY (batch_id, report_id)
);
CREATE INDEX issue_report_export_created_idx ON issue_report_export (created_at);

-- +goose Down
DROP TABLE issue_report_export;
DROP TABLE issue_report;
