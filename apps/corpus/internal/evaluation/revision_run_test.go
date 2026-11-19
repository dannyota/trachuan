package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func revisedSuites(t *testing.T) (Suite, Suite) {
	t.Helper()
	previous := periodSuite()
	previous.Development = previous.Development[:2]
	moving := Case{ID: "moving", Question: "Câu hỏi tốt", ExpectedCitations: []Expectation{{DocNumber: "doc"}}, Tags: []string{"multi_document"}}
	previous.Development = append(previous.Development, moving)
	entry := previous.Map.Entries[0]
	entry.CaseID = "moving"
	previous.Map.Entries = append(previous.Map.Entries, entry)
	data, err := json.Marshal(previous.Development)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	previous.Hashes.Development = hex.EncodeToString(sum[:])
	current := previous
	current.Development = append([]Case(nil), previous.Development...)
	current.Development[0].Tags = []string{"identifier", "transition"}
	current.Development[2].Tags = []string{"historical", "multi_document"}
	current.Hashes.Development = "revised-development"
	current.PreviousDevelopment = map[string][]byte{previous.Hashes.Development: data}
	return previous, current
}

func previousReportPath(t *testing.T, r Report) string {
	t.Helper()
	dir := t.TempDir()
	if err := r.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "report.json")
}

func TestRun_RevisedDevelopmentFixture(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	previousSuite, currentSuite := revisedSuites(t)
	baseline, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, previousSuite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	path := previousReportPath(t, baseline)
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, currentSuite, Options{EvaluationDate: date, PreviousReport: path})
	if err != nil {
		t.Fatal(err)
	}
	if !r.QualityPass {
		t.Fatal("tag-only revision failed quality")
	}
	for _, mode := range []retrieval.Mode{"exact", "ann"} {
		for _, spec := range []struct {
			name  string
			limit int
		}{{"recall", 20}, {"mrr", 8}} {
			n := developmentRecord(t, r, mode, spec.name, spec.limit)
			if n.Revision == nil || n.Revision.Cases != 2 || !reflect.DeepEqual(n.Revision.Revised, []RevisedCase{{"moving", "cohort"}}) || n.Status != "pass" {
				t.Fatalf("%s %s: %+v revision %+v", mode, spec.name, n, n.Revision)
			}
		}
	}
}

func TestRun_RevisedDevelopmentFixtureRefusals(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	previousSuite, _ := revisedSuites(t)
	baseline, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, previousSuite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	path := previousReportPath(t, baseline)
	for _, tc := range []struct {
		name   string
		mutate func(*Suite)
		want   string
	}{
		{"missing previous fixture", func(s *Suite) { s.PreviousDevelopment = nil }, "previous development fixture"},
		{"previous fixture hash differs", func(s *Suite) {
			for hash, data := range s.PreviousDevelopment {
				s.PreviousDevelopment[hash] = append(append([]byte(nil), data...), '\n')
			}
		}, "previous development fixture"},
		{"removed case", func(s *Suite) { s.Development = s.Development[:2] }, "removes case moving"},
		{"renamed case", func(s *Suite) { s.Development[2].ID = "renamed" }, "removes case moving"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, current := revisedSuites(t)
			tc.mutate(&current)
			if tc.name == "renamed case" {
				current.Map.Entries[len(current.Map.Entries)-1].CaseID = "renamed"
			}
			r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, current, Options{EvaluationDate: date, PreviousReport: path})
			if err == nil || !strings.Contains(err.Error(), tc.want) || r.Status != "" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRun_UnchangedFixtureWithDifferentCohortsRefused(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	previousSuite, _ := revisedSuites(t)
	baseline, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, previousSuite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	path := previousReportPath(t, baseline)
	current := previousSuite
	current.Development = append([]Case(nil), previousSuite.Development...)
	current.Development[2].Tags = []string{"historical"}
	if _, _, err = Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, current, Options{EvaluationDate: date, PreviousReport: path}); err == nil {
		t.Fatal("unchanged fixture hash with a different cohort accepted")
	}
}

func TestRun_VersionThreeScorerSix(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != 3 || r.Scorer != "agent-eval/6" || ScorerVersion != "agent-eval/6" {
		t.Fatalf("version %d scorer %s", r.Version, r.Scorer)
	}
}

func versionTwoReport(t *testing.T, r Report, scorer string) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["version"], object["scorer"] = 2, scorer
	if scorer != "agent-eval/5" {
		object["version"] = 1
	}
	delete(object["cohorts"].(map[string]any), "development_period")
	delete(object["not_run"].(map[string]any), "development_period")
	data, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun_PreviousScorerPairs(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	baseline, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date, PreviousReport: versionTwoReport(t, baseline, "agent-eval/5")})
	if err != nil {
		t.Fatal(err)
	}
	diff := r.PreviousDiff.(map[string]any)
	if diff["previous_scorer"] != "agent-eval/5" || diff["scorer"] != "agent-eval/6" || diff["previous"] == nil {
		t.Fatalf("diff = %v", diff)
	}
	for _, n := range readDevelopmentSummary(t, r).NonRegression {
		if n.Status != "pass" || n.PreviousValue == nil {
			t.Fatalf("version two comparison = %+v", n)
		}
	}
	for _, scorer := range []string{"agent-eval/1", "agent-eval/2", "agent-eval/3", "agent-eval/4"} {
		r, _, err = Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date, PreviousReport: versionTwoReport(t, baseline, scorer)})
		if err != nil {
			t.Fatal(err)
		}
		if diff := r.PreviousDiff.(map[string]any); diff["reason"] != "incompatible scorer version" {
			t.Fatalf("%s: diff = %v", scorer, diff)
		}
	}
}

func TestRun_HistoricalPolicyRemoved(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	r, cache, err := Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, periodSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = r.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	if err = cache.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(data), "historical_policy") {
			t.Fatalf("%s carries historical_policy", entry.Name())
		}
	}
	previous, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`null`, `"agent"`} {
		marked := strings.Replace(string(previous), `"version":3`, `"historical_policy":`+value+`,"version":3`, 1)
		marked = strings.Replace(marked, `"non_current_tail":null`, `"historical_policy":`+value+`,"non_current_tail":null`, 1)
		path := filepath.Join(t.TempDir(), "report.json")
		if err = os.WriteFile(path, []byte(marked), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err = Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, periodSuite(), Options{EvaluationDate: date, PreviousReport: path})
		if value == `null` && err != nil {
			t.Fatalf("null historical_policy refused: %v", err)
		}
		if value != `null` && (err == nil || !strings.Contains(err.Error(), "historical_policy")) {
			t.Fatalf("historical_policy %s: error %v", value, err)
		}
	}
}
