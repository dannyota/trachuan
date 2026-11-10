package build

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestCalibration_RejectsThresholdMismatch(t *testing.T) {
	calibration := []byte(`{"threshold":0.2}`)
	recipe := []byte(fmt.Sprintf(`{"abstention":{"threshold":0.7,"calibration_sha256":"%x"}}`, sha256.Sum256(calibration)))
	if err := verifyCalibration(recipe, calibration); err == nil {
		t.Fatal("accepted threshold mismatch")
	}
}

func TestRecord_ValidityCounts(t *testing.T) {
	counts := map[string]int64{"document": 2}
	addValidityCounts(counts, validityCounts{basis: map[string]int{"source_status": 1, "derived_from_text": 1}, ruleA: 1, ruleB: 2, ruleC: 3, review: map[string]int{"rule_b_skipped": 4}})
	for k, want := range map[string]int64{"documents_by_status_basis_source_status": 1, "documents_by_status_basis_derived_from_text": 1, "rule_a_corrections": 1, "rule_b_events": 2, "rule_c_conflicts": 3, "validity_review_rule_b_skipped": 4} {
		if counts[k] != want {
			t.Errorf("%s=%d want %d", k, counts[k], want)
		}
	}
}
