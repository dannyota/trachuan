-- +goose Up
CREATE INDEX run_membership_evidence_item_idx ON run_membership_evidence (run_id, stage, item_key);
CREATE INDEX run_seed_observation_item_idx ON run_seed_observation (run_id, stage, item_key);

-- +goose Down
DROP INDEX run_seed_observation_item_idx;
DROP INDEX run_membership_evidence_item_idx;
