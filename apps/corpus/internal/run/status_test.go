package run

import (
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

func TestStatus_ShowsQualityFields(t *testing.T) {
	q, r, err := qualityFields([]byte(`{"status":"complete","quality_pass":false,"release_eligible":false}`))
	if err != nil || q == nil || *q || r == nil || *r {
		t.Fatalf("quality fields: %v %v %v", q, r, err)
	}
}

func TestStatus_MissingEvaluationArtifactDoesNotFail(t *testing.T) {
	path, hash := "missing", "absent"
	report := Report{}
	report.loadQuality(artifact.Store{Root: t.TempDir()}, &path, &hash)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if report.QualityPass != nil || report.ReleaseEligible != nil || !strings.Contains(string(data), `"quality_reason"`) {
		t.Fatalf("status = %s", data)
	}
}
