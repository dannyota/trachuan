package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testRel     = "20990102_1"
	previousRel = "20990101_1"
)

type fakeQueries struct {
	store.Querier
	inventories map[int64]json.RawMessage
	runs        map[string]store.Run
	failure     error
}

func (f *fakeQueries) GetBuildInventory(_ context.Context, id int64) (json.RawMessage, error) {
	return f.inventories[id], f.failure
}

func (f *fakeQueries) GetRun(_ context.Context, arg store.GetRunParams) (store.Run, error) {
	row, ok := f.runs[arg.Country+"/"+arg.Rel]
	if !ok {
		return store.Run{}, pgx.ErrNoRows
	}
	return row, nil
}

type fixture struct {
	t         *testing.T
	env       stage.Env
	queries   *fakeQueries
	stage     *Stage
	run       stage.Run
	build     artifact.Result
	reportSum string
	report    map[string]any
	gaps      []gapRow
	previous  string
}

func newFixture(t *testing.T, decision stage.PreviousReport) *fixture {
	t.Helper()
	artifacts := artifact.Store{Root: t.TempDir()}
	f := &fixture{t: t, previous: previousRel, env: stage.Env{Artifacts: artifacts}, queries: &fakeQueries{inventories: map[int64]json.RawMessage{}, runs: map[string]store.Run{}}}
	f.run = stage.Run{ID: 2, Country: "vn", Rel: testRel, Snapshot: stage.Snapshot{PreviousReport: &decision}}
	f.build = f.publish("build", strings.Repeat("1", 64), map[string]string{"build.json": "{}"})
	f.report = map[string]any{"status": "complete", "country": "vn", "rel": testRel, "build_artifact_sha256": f.build.SHA256, "dump_sha256": strings.Repeat("d", 64), "previous_diff": map[string]any{"reason": "no previous report"}}
	f.stage = New(Config{})
	f.stage.queries = func(stage.Env) store.Querier { return f.queries }
	f.stage.lookup = func(context.Context, stage.Env, store.Querier, stage.Run) (artifact.Result, error) {
		return f.build, nil
	}
	f.stage.release = func(context.Context, stage.Env, stage.Run) (*pgxpool.Pool, func(), error) { return nil, func() {}, nil }
	f.sync()
	return f
}

func (f *fixture) publish(name, hash string, files map[string]string) artifact.Result {
	f.t.Helper()
	result, err := f.env.Artifacts.Publish(context.Background(), name, hash, hash, func(dir string) error {
		for file, body := range files {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func (f *fixture) sync() {
	f.t.Helper()
	data, err := json.Marshal(f.report)
	if err != nil {
		f.t.Fatal(err)
	}
	f.reportSum = bareSHA256(data)
	art := f.publish("eval", f.reportSum, map[string]string{"report.json": string(data)})
	key := f.run.Country + "/" + f.run.Rel
	f.queries.inventories[f.run.ID] = inventoryJSON(f.t, f.gaps, inventoryItemFor("build", key, f.build), inventoryItemFor("eval", key, art))
}

func inventoryItemFor(name, key string, art artifact.Result) inventoryItem {
	path, sum := art.Path, art.SHA256
	return inventoryItem{Stage: name, ItemKey: key, State: "done", Path: &path, ArtifactSHA256: &sum}
}

func inventoryJSON(t *testing.T, gaps []gapRow, items ...inventoryItem) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(inventory{Items: items, Gaps: gaps})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (f *fixture) comparedWith(sum string) {
	f.comparedIn("vn", sum)
}

func (f *fixture) comparedIn(country, sum string) {
	f.report["previous_diff"] = map[string]any{"previous": map[string]string{"country": country, "rel": f.previous, "report_sha256": sum}}
	f.sync()
}

func (f *fixture) process() (stage.Item, stage.Output, error) {
	f.t.Helper()
	items, err := f.stage.Plan(context.Background(), f.env, f.run)
	if err != nil {
		f.t.Fatal(err)
	}
	if len(items) != 1 || items[0].Key != "vn/"+testRel {
		f.t.Fatalf("items = %+v", items)
	}
	output, err := f.stage.Process(context.Background(), f.env, f.run, items[0])
	return items[0], output, err
}

func (f *fixture) findings(output stage.Output) File {
	f.t.Helper()
	file, _, err := readArtifactFindings(f.env.Artifacts.Abs(output.Artifact.Path), output.Artifact.SHA256)
	if err != nil {
		f.t.Fatal(err)
	}
	return file
}

func noneDecision() stage.PreviousReport { return stage.PreviousReport{SHA256: stage.NoPreviousReport} }

type fakeDetector struct {
	id          string
	observation Observation
	err         error
	seen        *Inputs
}

func (d fakeDetector) Spec() detector.Detector {
	return detector.Detector{ID: d.id, Version: 1, Semantics: 1, Severity: detector.Review}
}

func (d fakeDetector) Detect(_ context.Context, in Inputs) (Observation, error) {
	if d.seen != nil {
		*d.seen = in
	}
	return d.observation, d.err
}

func TestProcess_WritesBoundEmptyFindings(t *testing.T) {
	f := newFixture(t, noneDecision())
	item, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	if output.State != "done" || output.Artifact == nil || output.Artifact.InputHash != item.InputHash {
		t.Fatalf("output = %+v", output)
	}
	file := f.findings(output)
	want := Bindings{Rel: testRel, DumpSHA256: "sha256:" + strings.Repeat("d", 64), BuildArtifactSHA256: "sha256:" + f.build.SHA256, ReportSHA256: "sha256:" + f.reportSum, DetectorRegistrySHA256: "sha256:" + detector.Hash(), PredecessorRel: "none", PredecessorFindingsSHA256: "none"}
	if file.Bindings != want || file.Schema != 1 || len(file.Findings) != 0 || len(file.Detectors) != 0 {
		t.Fatalf("file = %+v", file)
	}
	entries, err := os.ReadDir(f.env.Artifacts.Abs(output.Artifact.Path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != findingsFile && entry.Name() != "MANIFEST.sha256" {
			t.Fatalf("unexpected artifact file %s", entry.Name())
		}
	}
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, *output.Artifact); err != nil {
		t.Fatal(err)
	}
}

func TestProcess_RepeatPublishesSameArtifact(t *testing.T) {
	f := newFixture(t, noneDecision())
	_, first, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifact.SHA256 != second.Artifact.SHA256 || !second.Artifact.Adopted {
		t.Fatalf("first=%+v second=%+v", first.Artifact, second.Artifact)
	}
}

func TestInputHash_CoversEveryFrozenInput(t *testing.T) {
	hashOf := func(mutate func(*fixture)) string {
		t.Helper()
		f := newFixture(t, noneDecision())
		if mutate != nil {
			mutate(f)
		}
		items, err := f.stage.Plan(context.Background(), f.env, f.run)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].InputHash
	}
	base := hashOf(nil)
	if base != hashOf(nil) {
		t.Fatal("input hash is not stable")
	}
	for name, mutate := range map[string]func(*fixture){
		"report": func(f *fixture) { f.report["dump_sha256"] = strings.Repeat("e", 64); f.sync() },
		"gaps":   func(f *fixture) { f.gaps = []gapRow{{"fetch", "a", "not_found", "x"}}; f.sync() },
		"knowledge": func(f *fixture) {
			if err := os.Chmod(f.env.Artifacts.Abs(f.build.Path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(f.env.Artifacts.Abs(f.build.Path), knowledgeFile), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"build": func(f *fixture) {
			f.build = f.publish("build", strings.Repeat("2", 64), map[string]string{"build.json": "{\"other\":1}"})
			f.report["build_artifact_sha256"] = f.build.SHA256
			f.sync()
		},
	} {
		if hashOf(mutate) == base {
			t.Errorf("%s does not change the input hash", name)
		}
	}
}

func TestInputHash_CoversPredecessorFindings(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	hashWith := func(file *File) string {
		t.Helper()
		f := newFixture(t, stage.PreviousReport{SHA256: frozen})
		f.comparedWith(frozen)
		f.predecessorRun(true, "evaluated", file)
		items, err := f.stage.Plan(context.Background(), f.env, f.run)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].InputHash
	}
	finding := Finding{Key: "sha256:" + strings.Repeat("1", 64), Detector: "cross", Subject: docSubject("vn:1"), Fingerprint: "sha256:" + strings.Repeat("2", 64), Semantics: 1, FirstRel: previousRel, Status: StatusOpen}
	if hashWith(predecessorFile(frozen)) == hashWith(predecessorFile(frozen, finding)) {
		t.Fatal("predecessor findings do not change the input hash")
	}
}

func TestGapSnapshot_IgnoresOrderAndEvidenceRows(t *testing.T) {
	first := inventory{Gaps: []gapRow{{"a", "1", "k", "d"}, {"b", "2", "k", "d"}}}
	second := inventory{Gaps: []gapRow{{"b", "2", "k", "d"}, {stage.EvidenceStage, "x", "k", "d"}, {"a", "1", "k", "d"}}}
	one, err := first.gapSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.gapSnapshot()
	if err != nil || one != two {
		t.Fatalf("snapshots differ: %s %s %v", one, two, err)
	}
}

func TestRecipe_CoversRegistryDetectorsAndSerialization(t *testing.T) {
	plain := New(Config{}).Recipe(stage.Snapshot{})
	if want := stage.RecipeHash(stage.EvidenceStage, "1", detector.Hash(), "[]", "findings.json/1"); plain != want {
		t.Fatalf("recipe = %s, want %s", plain, want)
	}
	if New(Config{Detectors: []Detector{fakeDetector{id: "x"}}}).Recipe(stage.Snapshot{}) == plain {
		t.Fatal("detector configuration not in recipe")
	}
}

func TestProcess_KnowledgeFileFailsUntilParserShips(t *testing.T) {
	f := newFixture(t, noneDecision())
	if err := os.WriteFile(filepath.Join(f.env.Artifacts.Abs(f.build.Path), knowledgeFile), []byte("entries: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.process()
	if err == nil || err.Error() != "detector_incomplete: knowledge parser unavailable" {
		t.Fatalf("err = %v", err)
	}
	assertPermanent(t, err)
}

func assertPermanent(t *testing.T, err error) {
	t.Helper()
	var retryable *stage.RetryableError
	var syntax *json.SyntaxError
	if errors.As(err, &retryable) || errors.As(err, &syntax) || errors.Is(err, stage.ErrSourceData) {
		t.Fatalf("failure would retry or become a gap: %v", err)
	}
}

func TestProcess_DetectorFindingsAndCounts(t *testing.T) {
	f := newFixture(t, noneDecision())
	var seen Inputs
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross", seen: &seen, observation: Observation{Findings: []Fired{fired("vn:1", "", "x")}, Observed: 3, NotObservable: 1, ObservedCompletely: always}}}
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	file := f.findings(output)
	if got := file.Detectors["cross"]; got != (DetectorCounts{Complete: true, Observed: 3, NotObservable: 1}) || len(file.Findings) != 1 {
		t.Fatalf("file = %+v", file)
	}
	if seen.Run.Rel != testRel || seen.BuildDir == "" || filepath.Base(seen.ReportPath) != "report.json" {
		t.Fatalf("inputs = %+v", seen)
	}
}

func TestProcess_FailedDetectorFailsItemNamingIt(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "good"}, fakeDetector{id: "broken", err: &json.SyntaxError{}}}
	_, output, err := f.process()
	if err == nil || !strings.HasPrefix(err.Error(), "detector_incomplete: broken") || output.Artifact != nil {
		t.Fatalf("output=%+v err=%v", output, err)
	}
	assertPermanent(t, err)
}

func TestProcess_ReleaseDatabaseFailureNamesDetectors(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross"}}
	f.stage.release = func(context.Context, stage.Env, stage.Run) (*pgxpool.Pool, func(), error) {
		return nil, nil, errors.New("connection refused")
	}
	_, _, err := f.process()
	if err == nil || !strings.HasPrefix(err.Error(), "detector_incomplete: cross") {
		t.Fatalf("err = %v", err)
	}
	assertPermanent(t, err)
}

func TestProcess_DuplicateDetectorNamesRefused(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "same"}, fakeDetector{id: "same"}}
	if _, _, err := f.process(); err == nil || !strings.Contains(err.Error(), "detector_incomplete: same") {
		t.Fatalf("err = %v", err)
	}
}

func TestProcess_NoDetectorsNeverOpensRelease(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.release = func(context.Context, stage.Env, stage.Run) (*pgxpool.Pool, func(), error) {
		t.Fatal("release database opened without a detector")
		return nil, nil, nil
	}
	if _, _, err := f.process(); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) predecessorRun(decided bool, state string, file *File) {
	f.t.Helper()
	snapshot := stage.Snapshot{}
	if decided {
		snapshot.PreviousReport = &stage.PreviousReport{SHA256: stage.NoPreviousReport}
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		f.t.Fatal(err)
	}
	f.queries.runs["vn/"+f.previous] = store.Run{ID: 1, Country: "vn", Rel: f.previous, State: state, Snapshot: raw}
	f.queries.inventories[1] = inventoryJSON(f.t, nil)
	if file == nil {
		return
	}
	data, err := file.encode()
	if err != nil {
		f.t.Fatal(err)
	}
	art := f.publish(stage.EvidenceStage, bareSHA256(data), map[string]string{findingsFile: string(data)})
	f.queries.inventories[1] = inventoryJSON(f.t, nil, inventoryItemFor(stage.EvidenceStage, "vn/"+f.previous, art))
}

func predecessorFile(reportSHA256 string, findings ...Finding) *File {
	file := File{Schema: 1, Bindings: Bindings{Rel: previousRel, ReportSHA256: "sha256:" + reportSHA256}, Detectors: map[string]DetectorCounts{}, Findings: findings, New: []string{}, Gone: []string{}, Carried: []string{}, Unmatched: []ReviewDecision{}}
	return &file
}

func TestProcess_OlderPredecessorHasEmptyFindings(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	f := newFixture(t, stage.PreviousReport{SHA256: frozen})
	f.previous = stage.LegacyCutoff("vn")
	f.comparedWith(frozen)
	f.predecessorRun(false, "released", nil)
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	got := f.findings(output).Bindings
	if got.PredecessorRel != f.previous || got.PredecessorFindingsSHA256 != "none" {
		t.Fatalf("bindings = %+v", got)
	}
}

func TestProcess_InheritsFromVerifiedPredecessor(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	f := newFixture(t, stage.PreviousReport{SHA256: frozen})
	f.comparedWith(frozen)
	subject := fired("vn:1", "", "x")
	key, err := findingKey("cross", subject)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := hashValue(subject.FingerprintInputs)
	if err != nil {
		t.Fatal(err)
	}
	prior := Finding{Key: key, Detector: "cross", Subject: subject.Subject, Fingerprint: fingerprint, Semantics: 1, FirstRel: "20990101", Status: StatusOpen}
	file := predecessorFile(frozen, prior)
	f.predecessorRun(true, "released", file)
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross", observation: Observation{Findings: []Fired{subject}, ObservedCompletely: always}}}
	item, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	got := f.findings(output)
	data, err := file.encode()
	if err != nil {
		t.Fatal(err)
	}
	if got.Findings[0].FirstRel != "20990101" || got.Bindings.PredecessorFindingsSHA256 != "sha256:"+bareSHA256(data) {
		t.Fatalf("file = %+v", got)
	}
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, *output.Artifact); err != nil {
		t.Fatal(err)
	}
}

func TestProcess_PredecessorRefusals(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name  string
		setup func(*fixture)
		want  string
	}{
		{"no run row", func(*fixture) {}, "has no run"},
		{"no decision above the cutoff", func(f *fixture) { f.predecessorRun(false, "released", nil) }, "no previous report decision"},
		{"other release bound", func(f *fixture) {
			file := predecessorFile(frozen)
			file.Bindings.Rel = "20990101_2"
			f.predecessorRun(true, "evaluated", file)
		}, "does not bind"},
		{"not evaluated", func(f *fixture) { f.predecessorRun(true, "paused", predecessorFile(frozen)) }, "not evaluated"},
		{"no evidence item", func(f *fixture) { f.predecessorRun(true, "evaluated", nil) }, "no done evidence"},
		{"other report bound", func(f *fixture) {
			f.predecessorRun(true, "evaluated", predecessorFile(strings.Repeat("b", 64)))
		}, "does not bind"},
		{"binding without prefix", func(f *fixture) {
			file := predecessorFile(frozen)
			file.Bindings.ReportSHA256 = frozen
			f.predecessorRun(true, "evaluated", file)
		}, "does not bind"},
		{"tampered artifact", func(f *fixture) {
			f.predecessorRun(true, "evaluated", predecessorFile(frozen))
			var inv inventory
			if err := json.Unmarshal(f.queries.inventories[1], &inv); err != nil {
				f.t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(f.env.Artifacts.Abs(*inv.Items[0].Path), findingsFile), []byte("{}"), 0600); err != nil {
				f.t.Fatal(err)
			}
		}, "failed verification"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, stage.PreviousReport{SHA256: frozen})
			f.comparedWith(frozen)
			tc.setup(f)
			item, output, err := f.process()
			if err == nil || !strings.Contains(err.Error(), tc.want) || output.Artifact != nil {
				t.Fatalf("output=%+v err=%v", output, err)
			}
			assertPermanent(t, err)
			if item.InputHash == "" {
				t.Fatal("refused input left the item without a hash")
			}
		})
	}
}

func TestProcess_ReportMustMatchRunAndDecision(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name     string
		decision stage.PreviousReport
		mutate   func(*fixture)
		want     string
	}{
		{"other build", noneDecision(), func(f *fixture) { f.report["build_artifact_sha256"] = strings.Repeat("9", 64); f.sync() }, "build"},
		{"other release", noneDecision(), func(f *fixture) { f.report["rel"] = previousRel; f.sync() }, "belong"},
		{"incomplete", noneDecision(), func(f *fixture) { f.report["status"] = "failed"; f.sync() }, "belong"},
		{"no dump hash", noneDecision(), func(f *fixture) { delete(f.report, "dump_sha256"); f.sync() }, "dump"},
		{"none with previous", noneDecision(), func(f *fixture) { f.comparedWith(frozen) }, "decision is none"},
		{"other previous", stage.PreviousReport{SHA256: frozen}, func(f *fixture) { f.comparedWith(strings.Repeat("b", 64)) }, "does not match"},
		{"no comparison", stage.PreviousReport{SHA256: frozen}, func(*fixture) {}, "does not match"},
		{"previous from another country", stage.PreviousReport{SHA256: frozen}, func(f *fixture) {
			f.predecessorRun(true, "evaluated", predecessorFile(frozen))
			f.comparedIn("us", frozen)
		}, "does not match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.decision)
			tc.mutate(f)
			if _, _, err := f.process(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestPlan_VerifiesDumpOnceProcessReadsManifest(t *testing.T) {
	f := newFixture(t, noneDecision())
	lookups := 0
	f.stage.lookup = func(context.Context, stage.Env, store.Querier, stage.Run) (artifact.Result, error) {
		lookups++
		return f.build, nil
	}
	item, output, err := f.process()
	if err != nil || lookups != 1 {
		t.Fatalf("lookups=%d err=%v", lookups, err)
	}
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, *output.Artifact); err != nil || lookups != 1 {
		t.Fatalf("lookups=%d err=%v", lookups, err)
	}
}

func TestPlan_RefusesVerifiedBuildOtherThanItem(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.lookup = func(context.Context, stage.Env, store.Querier, stage.Run) (artifact.Result, error) {
		return artifact.Result{SHA256: strings.Repeat("f", 64)}, nil
	}
	if _, err := f.stage.Plan(context.Background(), f.env, f.run); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("err = %v", err)
	}
}

func TestProcess_RunWithoutDecisionRefused(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.run.Snapshot.PreviousReport = nil
	if _, _, err := f.process(); err == nil || !strings.Contains(err.Error(), "no previous report decision") {
		t.Fatalf("err = %v", err)
	}
}

func TestProcess_InfrastructureFailureIsRetryable(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.queries.failure = errors.New("workshop down")
	_, err := f.stage.Process(context.Background(), f.env, f.run, stage.Item{Key: "vn/" + testRel})
	var retryable *stage.RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolve_RefusesStaleOrForeignArtifact(t *testing.T) {
	f := newFixture(t, noneDecision())
	item, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	stale := *output.Artifact
	stale.InputHash = strings.Repeat("7", 64)
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, stale); err == nil || !strings.Contains(err.Error(), "frozen inputs") {
		t.Fatalf("stale: %v", err)
	}
	wrong := *output.Artifact
	wrong.SHA256 = strings.Repeat("8", 64)
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, wrong); err == nil {
		t.Fatal("artifact with another manifest accepted")
	}
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, stage.Item{Key: "vn/other"}, *output.Artifact); err == nil {
		t.Fatal("other item accepted")
	}
}

func TestResolve_RefusesOtherBindingsUnderSameInputHash(t *testing.T) {
	f := newFixture(t, noneDecision())
	item, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	file := f.findings(output)
	file.Bindings.DumpSHA256 = "sha256:" + strings.Repeat("e", 64)
	data, err := file.encode()
	if err != nil {
		t.Fatal(err)
	}
	other := f.publish(stage.EvidenceStage, item.InputHash, map[string]string{findingsFile: string(data)})
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, other); err == nil || !strings.Contains(err.Error(), "bindings") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadFindings_ReadsOnlyTheRunsArtifact(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.predecessorRun(true, "evaluated", predecessorFile(strings.Repeat("a", 64)))
	file, err := LoadFindings(context.Background(), f.queries, f.env.Artifacts, "vn", previousRel)
	if err != nil || file.Bindings.Rel != previousRel {
		t.Fatalf("file=%+v err=%v", file, err)
	}
	if _, err = LoadFindings(context.Background(), f.queries, f.env.Artifacts, "vn", "20990105"); err == nil {
		t.Fatal("missing run accepted")
	}
}
