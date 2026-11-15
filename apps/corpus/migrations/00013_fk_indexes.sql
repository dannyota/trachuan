-- +goose Up
CREATE INDEX run_source_request_run_idx ON run_source_request (run_id);
CREATE INDEX fetch_baseline_run_idx ON fetch_baseline (run_id);
CREATE INDEX run_scope_predecessor_run_idx ON run_scope (predecessor_run_id);
CREATE INDEX run_source_event_source_idx ON run_source_event (run_id, source);
CREATE INDEX run_document_country_idx ON run_document (run_id, country);
CREATE INDEX run_item_artifact_idx ON run_item (artifact_id);
CREATE INDEX run_acquisition_artifact_idx ON run_acquisition (artifact_id);
CREATE INDEX fetch_baseline_artifact_idx ON fetch_baseline (artifact_id);
CREATE INDEX embed_row_artifact_idx ON embed_row (artifact_id);
CREATE INDEX fetched_file_document_idx ON fetched_file (artifact_id, source);

-- +goose Down
DROP INDEX fetched_file_document_idx;
DROP INDEX embed_row_artifact_idx;
DROP INDEX fetch_baseline_artifact_idx;
DROP INDEX run_acquisition_artifact_idx;
DROP INDEX run_item_artifact_idx;
DROP INDEX run_document_country_idx;
DROP INDEX run_source_event_source_idx;
DROP INDEX run_scope_predecessor_run_idx;
DROP INDEX fetch_baseline_run_idx;
DROP INDEX run_source_request_run_idx;
