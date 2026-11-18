package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/retrieval"
)

func previousReportFile(t *testing.T, mutate func(map[string]any)) (string, string) {
	t.Helper()
	recipe := retrieval.Recipe3()
	raw, err := recipe.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"version": 2, "status": "complete", "country": "vn", "rel": "20990101_1", "retrieval_recipe": value, "recipes": map[string]string{"retrieval": recipe.Hash()}}
	if mutate != nil {
		mutate(report)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

func decideInput(t *testing.T, choice ReportChoice, existing bool, snapshot *stage.Snapshot) decisionInput {
	t.Helper()
	return decisionInput{Choice: choice, Existing: existing, Country: "vn", Rel: "20990102_1", Snapshot: snapshot, Artifacts: artifact.Store{Root: t.TempDir()}, HasEvalItem: func(context.Context) (bool, error) { return false, nil }}
}

func TestDecide_NewRunRequiresExactlyOneChoice(t *testing.T) {
	path, _ := previousReportFile(t, nil)
	for name, choice := range map[string]ReportChoice{"neither": {}, "both": {Path: path, None: true}} {
		t.Run(name, func(t *testing.T) {
			snapshot := stage.Snapshot{}
			if _, err := decide(context.Background(), decideInput(t, choice, false, &snapshot)); err == nil || !strings.Contains(err.Error(), "--previous-report") {
				t.Fatalf("err = %v", err)
			}
			if snapshot.PreviousReport != nil {
				t.Fatal("refused start froze a decision")
			}
		})
	}
}

func TestDecide_NoPreviousReportFreezesNone(t *testing.T) {
	snapshot := stage.Snapshot{}
	if _, err := decide(context.Background(), decideInput(t, ReportChoice{None: true}, false, &snapshot)); err != nil {
		t.Fatal(err)
	}
	if want := (stage.PreviousReport{SHA256: stage.NoPreviousReport}); snapshot.PreviousReport == nil || *snapshot.PreviousReport != want {
		t.Fatalf("decision = %+v", snapshot.PreviousReport)
	}
}

func TestDecide_FreezesReportBytesInStore(t *testing.T) {
	path, sum := previousReportFile(t, nil)
	snapshot := stage.Snapshot{}
	in := decideInput(t, ReportChoice{Path: path}, false, &snapshot)
	if _, err := decide(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	decision := snapshot.PreviousReport
	if decision == nil || decision.SHA256 != sum || !strings.HasPrefix(decision.Path, "previous-report/") {
		t.Fatalf("decision = %+v", decision)
	}
	stored, err := os.ReadFile(filepath.Join(in.Artifacts.Abs(decision.Path), stage.PreviousReportFile))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil || string(stored) != string(original) {
		t.Fatalf("stored bytes differ: %v", err)
	}
	if _, err = artifact.Verify(in.Artifacts.Abs(decision.Path)); err != nil {
		t.Fatal(err)
	}
}

func TestDecide_RefusesUnusableReport(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"version", func(r map[string]any) { r["version"] = 1 }, "version 2"},
		{"status", func(r map[string]any) { r["status"] = "incomplete" }, "not complete"},
		{"recipe", func(r map[string]any) { delete(r, "retrieval_recipe") }, "retrieval recipe"},
		{"country", func(r map[string]any) { r["country"] = "us" }, "country"},
		{"same rel", func(r map[string]any) { r["rel"] = "20990102_1" }, "below"},
		{"later rel", func(r map[string]any) { r["rel"] = "20990103" }, "below"},
		{"bad rel", func(r map[string]any) { r["rel"] = "latest" }, "invalid release"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _ := previousReportFile(t, tc.mutate)
			snapshot := stage.Snapshot{}
			if _, err := decide(context.Background(), decideInput(t, ReportChoice{Path: path}, false, &snapshot)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDecide_ResumeWithoutFlagKeepsDecision(t *testing.T) {
	want := stage.PreviousReport{SHA256: strings.Repeat("a", 64), Path: "previous-report/x"}
	snapshot := stage.Snapshot{PreviousReport: &want}
	note, err := decide(context.Background(), decideInput(t, ReportChoice{}, true, &snapshot))
	if err != nil || note != "" || *snapshot.PreviousReport != want {
		t.Fatalf("note=%q decision=%+v err=%v", note, snapshot.PreviousReport, err)
	}
}

func TestDecide_SameDecisionIsNoChangeEvenAfterEval(t *testing.T) {
	path, sum := previousReportFile(t, nil)
	snapshot := stage.Snapshot{PreviousReport: &stage.PreviousReport{SHA256: sum, Path: "previous-report/original"}}
	in := decideInput(t, ReportChoice{Path: path}, true, &snapshot)
	in.HasEvalItem = func(context.Context) (bool, error) { return true, nil }
	note, err := decide(context.Background(), in)
	if err != nil || note != "" || snapshot.PreviousReport.Path != "previous-report/original" {
		t.Fatalf("note=%q decision=%+v err=%v", note, snapshot.PreviousReport, err)
	}
}

func TestDecide_ReplacesBeforeEvalAndRecordsIt(t *testing.T) {
	path, sum := previousReportFile(t, nil)
	snapshot := stage.Snapshot{PreviousReport: &stage.PreviousReport{SHA256: stage.NoPreviousReport}}
	note, err := decide(context.Background(), decideInput(t, ReportChoice{Path: path}, true, &snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PreviousReport.SHA256 != sum || !strings.Contains(note, "none") || !strings.Contains(note, sum) {
		t.Fatalf("note=%q decision=%+v", note, snapshot.PreviousReport)
	}
}

func TestDecide_RefusesDifferentDecisionOnceEvalItemExists(t *testing.T) {
	path, _ := previousReportFile(t, nil)
	original := stage.PreviousReport{SHA256: stage.NoPreviousReport}
	snapshot := stage.Snapshot{PreviousReport: &original}
	in := decideInput(t, ReportChoice{Path: path}, true, &snapshot)
	in.HasEvalItem = func(context.Context) (bool, error) { return true, nil }
	if _, err := decide(context.Background(), in); err == nil || !strings.Contains(err.Error(), "eval") {
		t.Fatalf("err = %v", err)
	}
	if *snapshot.PreviousReport != original {
		t.Fatalf("decision changed: %+v", snapshot.PreviousReport)
	}
	if entries, err := os.ReadDir(in.Artifacts.Root); err == nil && len(entries) != 0 {
		t.Fatalf("refused decision reached the store: %v", entries)
	}
}

func TestDecide_ReportsEvalItemLookupFailure(t *testing.T) {
	path, _ := previousReportFile(t, nil)
	sentinel := errors.New("workshop down")
	snapshot := stage.Snapshot{PreviousReport: &stage.PreviousReport{SHA256: stage.NoPreviousReport}}
	in := decideInput(t, ReportChoice{Path: path}, true, &snapshot)
	in.HasEvalItem = func(context.Context) (bool, error) { return false, sentinel }
	if _, err := decide(context.Background(), in); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestDecide_RefusesFlagOnRunWithoutDecision(t *testing.T) {
	path, _ := previousReportFile(t, nil)
	for name, choice := range map[string]ReportChoice{"report": {Path: path}, "none": {None: true}} {
		t.Run(name, func(t *testing.T) {
			snapshot := run57Snapshot()
			in := decideInput(t, choice, true, &snapshot)
			in.Rel = "20261118_1"
			if _, err := decide(context.Background(), in); err == nil || !strings.Contains(err.Error(), "predates") {
				t.Fatalf("err = %v", err)
			}
			if snapshot.PreviousReport != nil {
				t.Fatal("decision added to an older run")
			}
		})
	}
}

func TestDecide_MissingDecisionAboveCutoffNeedsFlag(t *testing.T) {
	snapshot := run57Snapshot()
	if _, err := decide(context.Background(), decideInput(t, ReportChoice{}, true, &snapshot)); err == nil || !strings.Contains(err.Error(), "--previous-report") {
		t.Fatalf("err = %v", err)
	}
	if snapshot.PreviousReport != nil {
		t.Fatalf("decision = %+v", snapshot.PreviousReport)
	}
}

func TestDecide_FlagAddsDecisionAboveCutoffBeforeEval(t *testing.T) {
	path, sum := previousReportFile(t, nil)
	for name, tc := range map[string]struct {
		choice ReportChoice
		want   string
	}{"report": {ReportChoice{Path: path}, sum}, "none": {ReportChoice{None: true}, stage.NoPreviousReport}} {
		t.Run(name, func(t *testing.T) {
			snapshot := run57Snapshot()
			note, err := decide(context.Background(), decideInput(t, tc.choice, true, &snapshot))
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.PreviousReport == nil || snapshot.PreviousReport.SHA256 != tc.want || !strings.Contains(note, tc.want) {
				t.Fatalf("note=%q decision=%+v", note, snapshot.PreviousReport)
			}
		})
	}
}

func TestDecide_FlagAboveCutoffRefusedOnceEvalItemExists(t *testing.T) {
	snapshot := run57Snapshot()
	in := decideInput(t, ReportChoice{None: true}, true, &snapshot)
	in.HasEvalItem = func(context.Context) (bool, error) { return true, nil }
	if _, err := decide(context.Background(), in); err == nil || !strings.Contains(err.Error(), "eval item") {
		t.Fatalf("err = %v", err)
	}
	if snapshot.PreviousReport != nil {
		t.Fatalf("decision = %+v", snapshot.PreviousReport)
	}
}

func TestDecide_OlderRunResumesWithoutFlag(t *testing.T) {
	snapshot := run57Snapshot()
	in := decideInput(t, ReportChoice{}, true, &snapshot)
	in.Rel = "20261118_1"
	if note, err := decide(context.Background(), in); err != nil || note != "" || snapshot.PreviousReport != nil {
		t.Fatalf("note=%q err=%v decision=%+v", note, err, snapshot.PreviousReport)
	}
}

func run57Snapshot() stage.Snapshot {
	names := []string{"discover", "expand", "fetch", "extract", "relate", "chunk", "embed", "build", "eval"}
	recipes := make(map[string]string, len(names))
	for _, name := range names {
		recipes[name] = "recipe-" + name
	}
	return stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":1}}`), SourceListHash: "h", Stages: names, Recipes: recipes, TaxonomyVersion: "none", WorkerVersion: "w"}
}

func TestExtendSnapshot_NeverAppendsEvidenceToRun57(t *testing.T) {
	snapshot := run57Snapshot()
	stages := append(append([]string{}, snapshot.Stages...), stage.EvidenceStage)
	recipes := map[string]string{stage.EvidenceStage: "recipe-evidence"}
	for name, recipe := range snapshot.Recipes {
		recipes[name] = recipe
	}
	got, changed, err := extendSnapshot(snapshot, includedStages(stages, "", len(snapshot.Stages)), recipes)
	if err != nil || changed || !reflect.DeepEqual(got.Stages, snapshot.Stages) || got.Recipes[stage.EvidenceStage] != "" {
		t.Fatalf("stages=%v changed=%v err=%v", got.Stages, changed, err)
	}
	if !stage.EvaluatedAfter("vn", "20261118_1", got, "eval") || stage.EvaluatedAfter("vn", "20261118_1", got, stage.EvidenceStage) {
		t.Fatal("older run must end at eval")
	}
	if err = stage.RequireDecision("vn", "20261118_1", got); err != nil {
		t.Fatalf("older run refused: %v", err)
	}
	if err = stage.RequireDecision("vn", "20261118_2", got); !errors.Is(err, stage.ErrNoDecision) {
		t.Fatalf("run above the cutoff accepted: %v", err)
	}
}

func TestExtendSnapshot_AppendsEvidenceWithDecision(t *testing.T) {
	snapshot := run57Snapshot()
	snapshot.PreviousReport = &stage.PreviousReport{SHA256: stage.NoPreviousReport}
	stages := append(append([]string{}, snapshot.Stages...), stage.EvidenceStage)
	recipes := map[string]string{stage.EvidenceStage: "recipe-evidence"}
	for name, recipe := range snapshot.Recipes {
		recipes[name] = recipe
	}
	got, changed, err := extendSnapshot(snapshot, stages, recipes)
	if err != nil || !changed || got.Stages[len(got.Stages)-1] != stage.EvidenceStage || got.Recipes[stage.EvidenceStage] != "recipe-evidence" {
		t.Fatalf("stages=%v changed=%v err=%v", got.Stages, changed, err)
	}
	if got.PreviousReport == nil || got.PreviousReport.SHA256 != stage.NoPreviousReport {
		t.Fatalf("decision lost: %+v", got.PreviousReport)
	}
}
