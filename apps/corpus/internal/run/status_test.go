package run

import (
	"testing"
)

func TestStatus_ShowsQualityFields(t *testing.T) {
	q, r, err := qualityFields([]byte(`{"status":"complete","quality_pass":false,"release_eligible":false}`))
	if err != nil || q == nil || *q || r == nil || *r {
		t.Fatalf("quality fields: %v %v %v", q, r, err)
	}
}
