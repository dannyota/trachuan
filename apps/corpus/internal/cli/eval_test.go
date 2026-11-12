package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

func TestEvalCLI_ExitCodes(t *testing.T) {
	for _, tc := range []struct {
		status  string
		require bool
		want    int
	}{{"pass", true, 0}, {"fail", true, 2}, {"unavailable", true, 2}, {"fail", false, 0}} {
		r := evaluation.Report{Status: "complete", QualityPass: true, Floors: make([]evaluation.Floor, 2)}
		for i := range r.Floors {
			r.Floors[i].Status = tc.status
			r.Floors[i].Mode = []retrieval.Mode{"exact", "ann"}[i]
			r.Floors[i].Name = "current_law_precision"
			r.Floors[i].Arms = "hybrid"
			r.Floors[i].Limit = 8
		}
		if got := evalExit(r, tc.require); got != tc.want {
			t.Fatalf("%s %v: %d", tc.status, tc.require, got)
		}
	}
	if got := evalExit(evaluation.Report{Status: "incomplete"}, true); got != 1 {
		t.Fatalf("incomplete: %d", got)
	}
}

func TestEvalCLI_RequireFloorsRejectsFailedQuality(t *testing.T) {
	r := evaluation.Report{Status: "complete", Floors: make([]evaluation.Floor, 2)}
	for i := range r.Floors {
		r.Floors[i].Status = "pass"
		r.Floors[i].Mode = []retrieval.Mode{"exact", "ann"}[i]
		r.Floors[i].Name = "current_law_precision"
		r.Floors[i].Arms = "hybrid"
		r.Floors[i].Limit = 8
	}
	r.QualityPass = true
	if got := evalExit(r, true); got != 0 {
		t.Fatalf("passing quality: got %d, want 0", got)
	}
	r.QualityPass = false
	if got := evalExit(r, true); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

type failingEvalCache struct{}

func (failingEvalCache) WriteFiles(string) error { return errors.New("cache write failed") }

func TestEvalOutput_AtomicAndRefusesNonempty(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "output")
	if err := writeEvaluationOutput(out, evaluation.Report{}, failingEvalCache{}); err == nil {
		t.Fatal("cache failure accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("partial output exists: %v", err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOutput(out, evaluation.Report{}, evaluation.Cache{}); err == nil {
		t.Fatal("nonempty output accepted")
	}
	if _, err := os.Stat(filepath.Join(out, "report.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected report: %v", err)
	}
}

func TestEvalOutput_WritesManifest(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	if err := writeEvaluationOutput(out, evaluation.Report{}, evaluation.Cache{}); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Verify(out); err != nil {
		t.Fatalf("verify output: %v", err)
	}
}

func TestEvalCLI_OpenDatabaseErrorIncludesRelease(t *testing.T) {
	_, err := openEvalDB(context.Background(), "vn", "20261109", "://invalid")
	if err == nil || !strings.Contains(err.Error(), "corpus eval vn/20261109") {
		t.Fatalf("error = %v", err)
	}
}

func TestEvalOutput_ExistingEmptyDirectory(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOutput(out, evaluation.Report{}, evaluation.Cache{}); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Verify(out); err != nil {
		t.Fatal(err)
	}
}

func TestEvalCLI_RejectsNonemptyOutputBeforeEvaluation(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORPUS_DATABASE_URL", "")
	var stderr bytes.Buffer
	if code := runEval(context.Background(), []string{"vn", "20261109", "--out", out}, &stderr, &stderr); code != 1 || !strings.Contains(stderr.String(), "output directory is not empty") {
		t.Fatalf("code=%d error=%s", code, stderr.String())
	}
}

func TestEvalCLI_CapFlag(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, cap := range []string{"1", "4", "50", "none"} {
		var stderr bytes.Buffer
		code := runEval(context.Background(), []string{"vn", "20261109", "--out", out, "--cap", cap}, &stderr, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "output directory is not empty") {
			t.Errorf("cap %s: code=%d error=%s", cap, code, stderr.String())
		}
	}
	for _, cap := range []string{"0", "51", "-1", "NONE", "1.5", "04"} {
		var stderr bytes.Buffer
		code := runEval(context.Background(), []string{"vn", "20261109", "--out", out, "--cap", cap}, &stderr, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "invalid cap") {
			t.Errorf("cap %s: code=%d error=%s", cap, code, stderr.String())
		}
	}
}

func TestEvalCLI_ValidityFlags(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--historical-policy", "agent"}, {"--non-current-tail", "0"}, {"--non-current-tail", "5"}, {"--not-yet-eligible"}} {
		var stderr bytes.Buffer
		input := append([]string{"vn", "20261109", "--out", out}, args...)
		if code := runEval(context.Background(), input, &stderr, &stderr); code != 1 || !strings.Contains(stderr.String(), "output directory is not empty") {
			t.Errorf("args %v: code=%d error=%s", args, code, stderr.String())
		}
	}
	for _, args := range [][]string{{"--historical-policy", "other"}, {"--non-current-tail", "-1"}, {"--non-current-tail", "6"}, {"--non-current-tail", "01"}} {
		var stderr bytes.Buffer
		input := append([]string{"vn", "20261109", "--out", out}, args...)
		if code := runEval(context.Background(), input, &stderr, &stderr); code != 1 || strings.Contains(stderr.String(), "output directory is not empty") {
			t.Errorf("invalid args %v: code=%d error=%s", args, code, stderr.String())
		}
	}
	for _, tc := range []struct {
		name  string
		flags []string
		rank  int
	}{
		{"historical", []string{"--historical-policy", "agent"}, 8},
		{"tail", []string{"--non-current-tail", "1"}, 8},
		{"not_yet", []string{"--not-yet-eligible"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepare := func(context.Context, string, string, string) (evalTarget, error) {
				return evalTarget{runner: cliValidityStage{kind: tc.name}, close: func() {}}, nil
			}
			result := filepath.Join(t.TempDir(), "report")
			var stderr bytes.Buffer
			args := append([]string{"vn", "20261109", "--out", result}, tc.flags...)
			if code := runEvalWith(context.Background(), args, &stderr, prepare); code != 0 {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			data, err := os.ReadFile(filepath.Join(result, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var report evaluation.Report
			if err = json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if (tc.name == "historical" && (report.HistoricalPolicy == nil || *report.HistoricalPolicy != "agent")) || (tc.name == "tail" && (report.NonCurrentTail == nil || *report.NonCurrentTail != 1)) || (tc.name == "not_yet" && !report.NotYetEligible) {
				t.Fatalf("report policy %+v", report.ValidityOverrides)
			}
			for _, row := range report.Cases {
				if row.CaseID == "positive" && row.Cohort == "legacy" && row.Arms == "hybrid" && row.Limit == 8 {
					if len(row.Ranks) != 1 || row.Ranks[0] != tc.rank || row.Matched != 1 || len(row.Returned) < tc.rank || row.Returned[tc.rank-1].DocumentKey != "target" {
						t.Fatalf("row %+v", row)
					}
					return
				}
			}
			t.Fatal("case missing")
		})
	}
}

type replaceOutputWithFile struct{ out string }

func (w replaceOutputWithFile) WriteFiles(string) error {
	if err := os.Remove(w.out); err != nil {
		return err
	}
	return os.WriteFile(w.out, []byte("keep"), 0600)
}

func TestEvalOutput_PreservesFileCreatedDuringEvaluation(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOutput(out, replaceOutputWithFile{out: out}, evaluation.Cache{}); err == nil {
		t.Fatal("accepted file as output directory")
	}
	data, err := os.ReadFile(out)
	if err != nil || string(data) != "keep" {
		t.Fatalf("file = %q, error = %v", data, err)
	}
}

func TestEvalCLI_RejectsDuplicateGates(t *testing.T) {
	r := evaluation.Report{Status: "complete", QualityPass: true, Floors: make([]evaluation.Floor, 2)}
	for i := range r.Floors {
		r.Floors[i] = evaluation.Floor{Metric: evaluation.Metric{Mode: "exact", Arms: "hybrid", Limit: 8, Name: "current_law_precision"}, Status: "pass"}
	}
	if got := evalExit(r, true); got != 2 {
		t.Fatalf("duplicate gates exit %d", got)
	}
}

func TestEvalCLI_RequireFloorsDevelopment(t *testing.T) {
	floors := []evaluation.Floor{
		{Metric: evaluation.Metric{Mode: "exact", Arms: "hybrid", Limit: 8, Name: "current_law_precision"}, Status: "pass"},
		{Metric: evaluation.Metric{Mode: "ann", Arms: "hybrid", Limit: 8, Name: "current_law_precision"}, Status: "pass"},
	}
	for _, tc := range []struct {
		name, status string
		count, want  int
	}{
		{"pass", "pass", 2, 0}, {"fail", "fail", 2, 2}, {"unavailable", "unavailable", 2, 2},
		{"empty", "not_run", 0, 0}, {"populated not_run", "not_run", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			development := append([]evaluation.Floor(nil), floors...)
			for i := range development {
				development[i].Status = tc.status
			}
			r := evaluation.Report{Status: "complete", QualityPass: true, Floors: floors, Cohorts: map[string]evaluation.Cohort{"development": {CaseCount: tc.count, Floors: development}}}
			if got := evalExit(r, true); got != tc.want {
				t.Fatalf("exit = %d, want %d", got, tc.want)
			}
			if got := evalExit(r, false); got != 0 {
				t.Fatalf("diagnostic exit = %d", got)
			}
		})
	}
	r := evaluation.Report{Status: "complete", QualityPass: true, Floors: floors, Rejections: []evaluation.Rejection{{Passed: false}}}
	if got := evalExit(r, true); got != 2 {
		t.Fatalf("failed rejection exit = %d", got)
	}
}

type cliCapEngine struct{ date retrieval.Date }

func (e cliCapEngine) Release() retrieval.Release {
	r := retrieval.Recipe1()
	threshold := 0.0
	r.Abstention.Threshold = &threshold
	return retrieval.Release{Country: "vn", Rel: "test", RetrievalRecipe: r}
}
func (e cliCapEngine) Prepare(_ context.Context, req retrieval.Request) (retrieval.Prepared, error) {
	v, err := retrieval.ValidateQuery(req.Query)
	if err != nil {
		return retrieval.Prepared{}, err
	}
	p := retrieval.Prepared{Request: req, Validation: v}
	if v.Accepted {
		p.Vector = &embedding.Vector{Values: []float32{1}, TokenCount: 1, RecipeHash: "query"}
	}
	return p, nil
}
func (e cliCapEngine) PrepareLegacy(ctx context.Context, req retrieval.Request) (retrieval.Prepared, error) {
	p, err := e.Prepare(ctx, req)
	if err == nil && !p.Validation.Accepted {
		p.Validation.Accepted = true
		p.Vector = &embedding.Vector{Values: []float32{1}, TokenCount: 1, RecipeHash: "query"}
	}
	return p, err
}
func (e cliCapEngine) Collect(_ context.Context, p retrieval.Prepared, depths retrieval.Depths) (retrieval.Arms, error) {
	var candidates []retrieval.Candidate
	for i := int64(1); i <= 10; i++ {
		doc := "other"
		if i <= 5 {
			doc = "doc"
		}
		id := retrieval.Identity{Country: "vn", DocumentKey: doc, Anchor: fmt.Sprint(i)}
		provision := retrieval.Provision{ID: i, Identity: id, NodeID: fmt.Sprint(i), Kind: "khoan", Text: "text"}
		candidates = append(candidates, retrieval.Candidate{ChunkID: i, GroupID: i, ProvisionID: i, Identity: id, Group: id, Cosine: .9, Provision: provision, Ancestors: []retrieval.Provision{provision}, Descendants: []retrieval.Provision{provision}, Document: retrieval.Document{Number: doc, StatusClass: "in_force", IsCurrent: true}})
	}
	return retrieval.Arms{Validation: p.Validation, Depths: depths, Date: e.date, Primary: retrieval.ArmGroup{Dense: candidates, Lexical: candidates, Members: candidates}}, nil
}
func (e cliCapEngine) CollectWithNotYetEligible(ctx context.Context, p retrieval.Prepared, depths retrieval.Depths) (retrieval.Arms, error) {
	return e.Collect(ctx, p, depths)
}
func (e cliCapEngine) Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{}, nil
}

type cliCapStage struct{}

type cliValidityEngine struct {
	cliCapEngine
	kind string
}

func (e cliValidityEngine) collect(p retrieval.Prepared, d retrieval.Depths, notYet bool) retrieval.Arms {
	arms := retrieval.Arms{Validation: p.Validation, Depths: d, Date: e.date}
	if p.Request.Query != "Câu hỏi tốt" {
		return arms
	}
	count := 7
	if e.kind == "tail" {
		count = 8
	}
	var current []retrieval.Candidate
	for i := 1; i <= count; i++ {
		current = append(current, cliValidityCandidate(int64(i), fmt.Sprintf("current-%d", i), "in_force"))
	}
	other := cliValidityCandidate(50, "target", "expired")
	if notYet {
		other.Document.StatusClass = "not_yet"
		current = append([]retrieval.Candidate{other}, current...)
	}
	arms.Primary = retrieval.ArmGroup{Dense: current, Lexical: current, Members: current}
	if p.Request.InForceOnly != nil && !*p.Request.InForceOnly {
		arms.IncludeNonCurrent = true
		if !notYet {
			arms.NonCurrent = retrieval.ArmGroup{Dense: []retrieval.Candidate{other}, Lexical: []retrieval.Candidate{other}, Members: []retrieval.Candidate{other}}
		}
	}
	return arms
}
func cliValidityCandidate(id int64, doc, status string) retrieval.Candidate {
	identity := retrieval.Identity{Country: "vn", DocumentKey: doc, Anchor: "one"}
	p := retrieval.Provision{ID: id, Identity: identity, NodeID: fmt.Sprint(id), Kind: "khoan", Text: "text"}
	return retrieval.Candidate{ChunkID: id, GroupID: id, ProvisionID: id, Identity: identity, Group: identity, Cosine: .9, Provision: p, Ancestors: []retrieval.Provision{p}, Descendants: []retrieval.Provision{p}, Document: retrieval.Document{Number: doc, StatusClass: status, IsCurrent: status == "in_force"}}
}
func (e cliValidityEngine) Collect(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	return e.collect(p, d, false), nil
}
func (e cliValidityEngine) CollectWithNotYetEligible(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	return e.collect(p, d, true), nil
}

type cliValidityStage struct{ kind string }

func (s cliValidityStage) Evaluate(ctx context.Context, _ stage.Env, _ stage.Run, opts evaluation.Options) (evaluation.Report, evaluation.Cache, error) {
	cases := []evaluation.Case{{ID: "positive", Question: "Câu hỏi tốt", Tags: []string{"historical"}, ExpectedCitations: []evaluation.Expectation{{DocNumber: "target"}}}, {ID: "reject1", Question: "khong dau mot", ExpectedCitations: []evaluation.Expectation{{DocNumber: "target"}}}, {ID: "reject2", Question: "khong dau hai", ExpectedCitations: []evaluation.Expectation{{DocNumber: "target"}}}, {ID: "reject3", Question: "khong dau ba", ExpectedCitations: []evaluation.Expectation{{DocNumber: "target"}}}}
	suite := evaluation.Suite{Legacy: cases, Rejections: []string{"reject1", "reject2", "reject3"}, Hashes: evaluation.FixtureHashes{Legacy: "fixture"}, Map: evaluation.IdentityMap{Version: 1, LegacySHA256: "fixture"}}
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	opts.EvaluationDate, opts.Modes = date, []retrieval.Mode{"exact"}
	return evaluation.Run(ctx, cliValidityEngine{cliCapEngine{date}, s.kind}, suite, opts)
}

func (cliCapStage) Evaluate(ctx context.Context, _ stage.Env, _ stage.Run, opts evaluation.Options) (evaluation.Report, evaluation.Cache, error) {
	cases := []evaluation.Case{{ID: "positive", Question: "Câu hỏi tốt", ExpectedCitations: []evaluation.Expectation{{DocNumber: "doc"}}}, {ID: "reject1", Question: "khong dau mot", ExpectedCitations: []evaluation.Expectation{{DocNumber: "doc"}}}, {ID: "reject2", Question: "khong dau hai", ExpectedCitations: []evaluation.Expectation{{DocNumber: "doc"}}}, {ID: "reject3", Question: "khong dau ba", ExpectedCitations: []evaluation.Expectation{{DocNumber: "doc"}}}}
	suite := evaluation.Suite{Legacy: cases, Rejections: []string{"reject1", "reject2", "reject3"}, Hashes: evaluation.FixtureHashes{Legacy: "fixture"}, Map: evaluation.IdentityMap{Version: 1, LegacySHA256: "fixture"}}
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	opts.EvaluationDate, opts.Modes = date, []retrieval.Mode{"exact"}
	return evaluation.Run(ctx, cliCapEngine{date}, suite, opts)
}

func TestEvalCLI_CapForwardedToResults(t *testing.T) {
	prepare := func(context.Context, string, string, string) (evalTarget, error) {
		return evalTarget{runner: cliCapStage{}, close: func() {}}, nil
	}
	var results []evaluation.Report
	for _, value := range []string{"3", "none"} {
		out := filepath.Join(t.TempDir(), "report")
		var stderr bytes.Buffer
		code := runEvalWith(context.Background(), []string{"vn", "20261109", "--out", out, "--cap", value}, &stderr, prepare)
		if code != 0 {
			t.Fatalf("cap %s: exit %d: %s", value, code, stderr.String())
		}
		b, err := os.ReadFile(filepath.Join(out, "report.json"))
		if err != nil {
			t.Fatal(err)
		}
		var report evaluation.Report
		if err = json.Unmarshal(b, &report); err != nil {
			t.Fatal(err)
		}
		if report.CapOverride == nil {
			t.Fatal("report omitted cap")
		}
		results = append(results, report)
	}
	if *results[0].CapOverride != 3 || *results[1].CapOverride != 0 {
		t.Fatalf("caps: %v, %v", results[0].CapOverride, results[1].CapOverride)
	}
	if len(results[0].Cases) == 0 || len(results[1].Cases) == 0 || len(results[0].Cases[0].Returned) < 4 || len(results[1].Cases[0].Returned) < 4 {
		t.Fatal("missing returned hits")
	}
	if results[0].Cases[0].Returned[3].DocumentKey != "other" || results[1].Cases[0].Returned[3].DocumentKey != "doc" {
		t.Fatalf("baseline %v trial %v", results[0].Cases[0].Returned, results[1].Cases[0].Returned)
	}
}

func TestEvalCLI_RequireFloorsIgnoresAbstention(t *testing.T) {
	floors := []evaluation.Floor{{Metric: evaluation.Metric{Mode: "exact", Arms: "hybrid", Limit: 8, Name: "current_law_precision"}, Status: "pass"}, {Metric: evaluation.Metric{Mode: "ann", Arms: "hybrid", Limit: 8, Name: "current_law_precision"}, Status: "pass"}}
	r := evaluation.Report{Status: "complete", QualityPass: true, Floors: floors, Targets: []evaluation.Target{{Metric: evaluation.Metric{Name: "abstention_balanced_accuracy"}, Status: "below"}}}
	if got := evalExit(r, true); got != 0 {
		t.Fatalf("exit = %d", got)
	}
}
