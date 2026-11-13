-- +goose Up
ALTER TABLE run_membership_evidence DROP CONSTRAINT run_membership_evidence_rule_kind_check;
ALTER TABLE run_membership_evidence ADD CONSTRAINT run_membership_evidence_rule_kind_check CHECK (rule_kind IN ('agency', 'term', 'named', 'pinned', 'secondary_probe', 'relation'));
ALTER TABLE run_fetch_observation DROP CONSTRAINT run_fetch_observation_acquisition_check;
ALTER TABLE run_fetch_observation ADD CONSTRAINT run_fetch_observation_acquisition_check CHECK (acquisition IN ('network', 'cache', 'adopted', 'pinned'));

-- +goose Down
ALTER TABLE run_fetch_observation DROP CONSTRAINT run_fetch_observation_acquisition_check;
ALTER TABLE run_fetch_observation ADD CONSTRAINT run_fetch_observation_acquisition_check CHECK (acquisition IN ('network', 'cache', 'adopted'));
ALTER TABLE run_membership_evidence DROP CONSTRAINT run_membership_evidence_rule_kind_check;
ALTER TABLE run_membership_evidence ADD CONSTRAINT run_membership_evidence_rule_kind_check CHECK (rule_kind IN ('agency', 'term', 'named', 'secondary_probe', 'relation'));
