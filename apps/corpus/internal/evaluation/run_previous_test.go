package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestPreviousDiff_IdentityDoesNotChangeQualityGates(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprint(populated), func(t *testing.T) {
			testPreviousIdentityGates(t, populated)
		})
	}
}

func testPreviousIdentityGates(t *testing.T, populated bool) {
	t.Helper()
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	opts := Options{EvaluationDate: date}
	newEngine := func() Engine {
		if populated {
			return &recordedEngine{fakeEngine: fakeEngine{date: date}}
		}
		return &fakeEngine{date: date}
	}
	baseline, _, err := Run(context.Background(), newEngine(), recordedSuite(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.QualityPass != populated {
		t.Fatalf("quality_pass=%v, populated=%v", baseline.QualityPass, populated)
	}
	prior := baseline
	prior.Country, prior.Rel = "vn", "20990101_1"
	data, err := json.MarshalIndent(prior, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	opts.PreviousReport = filepath.Join(t.TempDir(), "report.json")
	if err = os.WriteFile(opts.PreviousReport, data, 0600); err != nil {
		t.Fatal(err)
	}
	current, _, err := Run(context.Background(), newEngine(), recordedSuite(), opts)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	diff := current.PreviousDiff.(map[string]any)
	want := map[string]string{"country": prior.Country, "rel": prior.Rel, "report_sha256": hex.EncodeToString(sum[:])}
	encoded, err := json.Marshal(diff["previous"])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err = json.Unmarshal(encoded, &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("previous=%s want=%v err=%v", encoded, want, err)
	}
	if baseline.QualityPass != current.QualityPass || baseline.ReleaseEligible != current.ReleaseEligible ||
		!reflect.DeepEqual(baseline.Floors, current.Floors) || !reflect.DeepEqual(baseline.Rejections, current.Rejections) ||
		!reflect.DeepEqual(baseline.FilterGate, current.FilterGate) || !reflect.DeepEqual(baseline.CompactSizeGate, current.CompactSizeGate) {
		t.Fatal("previous report changed quality gates")
	}
	for name, cohort := range baseline.Cohorts {
		if !reflect.DeepEqual(cohort.Floors, current.Cohorts[name].Floors) {
			t.Fatalf("previous report changed %s floors", name)
		}
	}
	if _, ok := baseline.PreviousDiff.(map[string]any)["previous"]; ok {
		t.Fatal("identity present without a previous report")
	}
}
