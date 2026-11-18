package stage

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestRecipeHash_Stable(t *testing.T) {
	// SHA-256 of the bytes "fetch\x00v1\x00extra".
	const want = "c401cb8b0db5ef8c50f444720aaae299c9f288d0c7b411b97b920c268119f0b2"
	if got := RecipeHash("fetch", "v1", "extra"); got != want {
		t.Errorf("RecipeHash() = %q, want %q", got, want)
	}
}

func TestCompareRel_MissingSuffixSortsAsZero(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"20990101", "20990101_1", -1},
		{"20990101_1", "20990101", 1},
		{"20990101_2", "20990101_10", -1},
		{"20990102", "20990101_9", 1},
		{"20990101_0", "20990101", 0},
	} {
		if got := CompareRel(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareRel(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPredatesDecision_FollowsCutoff(t *testing.T) {
	decided := Snapshot{PreviousReport: &PreviousReport{SHA256: NoPreviousReport}}
	for _, tc := range []struct {
		country, rel string
		snapshot     Snapshot
		want         bool
	}{
		{"vn", "20261118_1", Snapshot{}, true},
		{"vn", "20261118", Snapshot{}, true},
		{"vn", "20261117_9", Snapshot{}, true},
		{"vn", "20261118_2", Snapshot{}, true},
		{"vn", "20261118_3", Snapshot{}, false},
		{"vn", "20261119", Snapshot{}, false},
		{"vn", "20990101", Snapshot{}, false},
		{"us", "20200101", Snapshot{}, false},
		{"vn", "20261109", decided, false},
	} {
		if got := PredatesDecision(tc.country, tc.rel, tc.snapshot); got != tc.want {
			t.Errorf("%s/%s: got %v, want %v", tc.country, tc.rel, got, tc.want)
		}
	}
}

func TestRequireDecision_RefusesMissingDecisionAboveCutoff(t *testing.T) {
	decided := Snapshot{PreviousReport: &PreviousReport{SHA256: NoPreviousReport}}
	if err := RequireDecision("vn", "20990101", Snapshot{}); !errors.Is(err, ErrNoDecision) {
		t.Fatalf("missing decision above cutoff: %v", err)
	}
	if err := RequireDecision("us", "20200101", Snapshot{}); !errors.Is(err, ErrNoDecision) {
		t.Fatalf("country without cutoff: %v", err)
	}
	for _, tc := range []struct {
		rel      string
		snapshot Snapshot
	}{{"20261118_1", Snapshot{}}, {"20990101", decided}} {
		if err := RequireDecision("vn", tc.rel, tc.snapshot); err != nil {
			t.Errorf("%s: %v", tc.rel, err)
		}
	}
}

func TestEvaluatedAfter_FollowsOlderRunRule(t *testing.T) {
	decided := Snapshot{PreviousReport: &PreviousReport{SHA256: strings.Repeat("a", 64), Path: "previous-report/x"}}
	for _, tc := range []struct {
		name, rel string
		snapshot  Snapshot
		stage     string
		want      bool
	}{
		{"older run at eval", "20261118_1", Snapshot{}, "eval", true},
		{"older run at build", "20261118_1", Snapshot{}, "build", false},
		{"older run at evidence", "20261118_1", Snapshot{}, EvidenceStage, false},
		{"decided run at eval", "20990101", decided, "eval", false},
		{"decided run at evidence", "20990101", decided, EvidenceStage, true},
		{"decided older rel at eval", "20261118_1", decided, "eval", false},
		{"missing decision above cutoff at eval", "20261118_3", Snapshot{}, "eval", false},
		{"missing decision above cutoff at evidence", "20261118_3", Snapshot{}, EvidenceStage, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluatedAfter("vn", tc.rel, tc.snapshot, tc.stage); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSnapshot_OldEncodingRoundTripsWithoutDecision(t *testing.T) {
	const old = `{"SourceList":{"country":"vn"},"SourceListHash":"h","Stages":["discover","eval"],"Recipes":{"discover":"d","eval":"e"},"TaxonomyVersion":"none","WorkerVersion":"w","PreviousManifest":""}`
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(old), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.PreviousReport != nil {
		t.Fatalf("decision = %+v", snapshot.PreviousReport)
	}
	again, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != old {
		t.Fatalf("re-encoded = %s", again)
	}
}

func TestSnapshot_DecisionRoundTrips(t *testing.T) {
	want := PreviousReport{SHA256: strings.Repeat("b", 64), Path: "previous-report/x"}
	data, err := json.Marshal(Snapshot{PreviousReport: &want})
	if err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err = json.Unmarshal(data, &got); err != nil || got.PreviousReport == nil || *got.PreviousReport != want {
		t.Fatalf("got %+v err=%v", got.PreviousReport, err)
	}
	if (PreviousReport{SHA256: NoPreviousReport}).None() != true || want.None() {
		t.Fatal("None misreads the decision")
	}
}

func TestLegacyCutoff_MatchesPythonRules(t *testing.T) {
	data, err := os.ReadFile("../../../../deploy/github/corpus_rules.py")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`LEGACY_CUTOFF = \{'vn': '([0-9_]+)'\}`).FindSubmatch(data)
	if match == nil {
		t.Fatal("LEGACY_CUTOFF not found in corpus_rules.py")
	}
	if got := string(match[1]); got != LegacyCutoff("vn") {
		t.Fatalf("python cutoff %s, go cutoff %s", got, LegacyCutoff("vn"))
	}
}

func TestClosedEvaluated_ReadsFrozenSnapshot(t *testing.T) {
	got, err := closedEvaluated([]byte(`{"PreviousReport":{"sha256":"none"}}`), "vn", "20990101", EvidenceStage)
	if err != nil || !got {
		t.Fatalf("decided run: %v %v", got, err)
	}
	if got, err = closedEvaluated([]byte(`{}`), "vn", "20261118_1", "eval"); err != nil || !got {
		t.Fatalf("older run: %v %v", got, err)
	}
	if got, err = closedEvaluated([]byte(`{}`), "vn", "20990101", "eval"); err != nil || got {
		t.Fatalf("missing decision above the cutoff: %v %v", got, err)
	}
	if _, err = closedEvaluated([]byte(`[`), "vn", "20261118_1", "eval"); err == nil {
		t.Fatal("malformed snapshot accepted")
	}
}
