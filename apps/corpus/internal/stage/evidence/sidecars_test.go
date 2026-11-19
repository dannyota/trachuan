package evidence

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (f *fixture) artifactFile(output stage.Output, name string) []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.env.Artifacts.Abs(output.Artifact.Path), name))
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}

func firstLine(t *testing.T, data []byte, out any) {
	t.Helper()
	line, _, _ := bytes.Cut(data, []byte("\n"))
	if err := strictDecode(line, out); err != nil {
		t.Fatal(err)
	}
}

func predecessorIdentityBytes(t *testing.T, rel, reportSHA256 string) []byte {
	t.Helper()
	src, m := previousSummary()
	b := identityBindings{Rel: rel, DumpSHA256: hashPrefix + strings.Repeat("e", 64), BuildArtifactSHA256: hashPrefix + strings.Repeat("f", 64), ReportSHA256: hashPrefix + reportSHA256}
	var buf bytes.Buffer
	if err := writeIdentity(context.Background(), &buf, src, m, b); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (f *fixture) predecessorIdentity(reportSHA256 string) []byte {
	f.t.Helper()
	return predecessorIdentityBytes(f.t, f.previous, reportSHA256)
}

func TestProcess_WritesThreeBoundFiles(t *testing.T) {
	f := newFixture(t, noneDecision())
	item, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(f.env.Artifacts.Abs(output.Artifact.Path))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, ",") != "MANIFEST.sha256,changes.jsonl,findings.json,identity.jsonl" {
		t.Fatalf("artifact files %v", names)
	}
	want := identityBindings{Rel: testRel, DumpSHA256: hashPrefix + strings.Repeat("d", 64), BuildArtifactSHA256: hashPrefix + f.build.SHA256, ReportSHA256: hashPrefix + f.reportSum}
	identity := f.artifactFile(output, identityFile)
	var ih identityHeader
	firstLine(t, identity, &ih)
	if ih.Schema != 1 || ih.Bindings != want {
		t.Fatalf("identity header %+v", ih)
	}
	var expected bytes.Buffer
	if err = writeIdentity(context.Background(), &expected, f.summary, f.members, want); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identity, expected.Bytes()) {
		t.Fatalf("identity differs from the summary of the release:\n%s", identity)
	}
	var ch changesHeader
	changes := f.artifactFile(output, changesFile)
	firstLine(t, changes, &ch)
	if ch != (changesHeader{Schema: 1, Bindings: changesBindings{identityBindings: want, PredecessorRel: "none", PredecessorIdentitySHA256: "none"}, Baseline: true}) || bytes.Count(changes, []byte("\n")) != 1 {
		t.Fatalf("changes %s", changes)
	}
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, *output.Artifact); err != nil {
		t.Fatal(err)
	}
}

func TestProcess_ChangesAgainstTheVerifiedPredecessorSummary(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	f := newFixture(t, stage.PreviousReport{SHA256: frozen})
	f.comparedWith(frozen)
	previous := f.predecessorIdentity(frozen)
	f.predecessorEvidence(predecessorFile(frozen), previous)
	item, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	var ch changesHeader
	changes := f.artifactFile(output, changesFile)
	firstLine(t, changes, &ch)
	if ch.Baseline || ch.Bindings.PredecessorRel != previousRel || ch.Bindings.PredecessorIdentitySHA256 != hashPrefix+bareSHA256(previous) {
		t.Fatalf("changes header %+v", ch)
	}
	if !bytes.Contains(changes, []byte(`{"type":"document_key","key":"vn:vbpl:2099004","before":"vn:chinhphu:2099003"}`)) {
		t.Fatalf("changes not compared with the predecessor:\n%s", changes)
	}
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, *output.Artifact); err != nil {
		t.Fatal(err)
	}
}

func TestProcess_PredecessorWithoutDecisionIsBaseline(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	f := newFixture(t, stage.PreviousReport{SHA256: frozen})
	f.previous = stage.LegacyCutoff("vn")
	f.comparedWith(frozen)
	f.predecessorRun(false, "released", nil)
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	var ch changesHeader
	firstLine(t, f.artifactFile(output, changesFile), &ch)
	if !ch.Baseline || ch.Bindings.PredecessorRel != f.previous || ch.Bindings.PredecessorIdentitySHA256 != "none" {
		t.Fatalf("changes header %+v", ch)
	}
}

func TestProcess_PredecessorSummaryRefusals(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name     string
		identity func(*fixture) []byte
		want     string
	}{
		{"no identity", func(*fixture) []byte { return nil }, "has no identity.jsonl"},
		{"other report", func(f *fixture) []byte { return f.predecessorIdentity(strings.Repeat("b", 64)) }, "does not bind"},
		{"other release", func(f *fixture) []byte {
			return bytes.Replace(f.predecessorIdentity(frozen), []byte(previousRel), []byte("20990101_2"), 1)
		}, "does not bind"},
		{"other schema", func(f *fixture) []byte {
			return bytes.Replace(f.predecessorIdentity(frozen), []byte(`"schema":1`), []byte(`"schema":2`), 1)
		}, "schema"},
		{"no header", func(*fixture) []byte { return []byte{} }, "header"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, stage.PreviousReport{SHA256: frozen})
			f.comparedWith(frozen)
			f.predecessorEvidence(predecessorFile(frozen), tc.identity(f))
			_, output, err := f.process()
			if err == nil || !strings.Contains(err.Error(), tc.want) || output.Artifact != nil {
				t.Fatalf("output=%+v err=%v", output, err)
			}
			assertPermanent(t, err)
		})
	}
}

func TestInputHash_CoversPredecessorSummary(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	hashWith := func(identity []byte) string {
		t.Helper()
		f := newFixture(t, stage.PreviousReport{SHA256: frozen})
		f.comparedWith(frozen)
		if identity == nil {
			identity = f.predecessorIdentity(frozen)
		}
		f.predecessorEvidence(predecessorFile(frozen), identity)
		items, err := f.stage.Plan(context.Background(), f.env, f.run)
		if err != nil {
			t.Fatal(err)
		}
		return items[0].InputHash
	}
	f := newFixture(t, noneDecision())
	other := append(f.predecessorIdentity(frozen), []byte(`{"type":"relation","kind":"cites","method":"x","source":{"key":"k","number":"n","in_release":false,"anchor":null},"target":{"key":null,"number":"m","in_release":false,"anchor":null},"count":1}`+"\n")...)
	if hashWith(nil) == hashWith(other) {
		t.Fatal("predecessor summary does not change the input hash")
	}
}

func TestProcess_MembershipFailureFailsTheItem(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.members = func(context.Context, stage.Env, stage.Run) (members, error) {
		return members{}, refuse("membership inventory differs from the build's")
	}
	_, output, err := f.process()
	if err == nil || !strings.Contains(err.Error(), "membership") || output.Artifact != nil {
		t.Fatalf("output=%+v err=%v", output, err)
	}
	assertPermanent(t, err)
}

func TestProcess_SummaryReadFailureRetries(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.summary.err = errors.New("connection reset")
	_, _, err := f.process()
	var retryable *stage.RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("err = %v", err)
	}
}

func TestProcess_ReleaseOpenedOnceForDetectorsAndSummary(t *testing.T) {
	for _, detectors := range [][]Detector{nil, {fakeDetector{id: "cross", observation: Observation{ObservedCompletely: always}}}} {
		f := newFixture(t, noneDecision())
		f.stage.cfg.Detectors = detectors
		opened := 0
		f.stage.release = func(context.Context, stage.Env, stage.Run) (*pgxpool.Pool, func(), error) {
			opened++
			return nil, func() {}, nil
		}
		if _, _, err := f.process(); err != nil || opened != 1 {
			t.Fatalf("detectors=%d opened=%d err=%v", len(detectors), opened, err)
		}
	}
}

func TestProcess_DetectorEntriesCarryRegistrySeverity(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross", observation: Observation{Findings: []Fired{fired("vn:1", "", "x")}, Observed: 3, ObservedCompletely: always}}}
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Schema    int                        `json:"schema"`
		Detectors map[string]json.RawMessage `json:"detectors"`
	}
	if err = json.Unmarshal(f.artifactFile(output, findingsFile), &raw); err != nil {
		t.Fatal(err)
	}
	var entry bytes.Buffer
	if err = json.Compact(&entry, raw.Detectors["cross"]); err != nil {
		t.Fatal(err)
	}
	if raw.Schema != 2 || entry.String() != `{"complete":true,"severity":"review","observed":3,"not_observable":0,"excluded":0}` {
		t.Fatalf("findings schema %d entry %s", raw.Schema, entry.String())
	}
}

func TestResolve_RefusesSidecarsBoundElsewhere(t *testing.T) {
	for name, mutate := range map[string]func([]byte) []byte{
		"identity": func(data []byte) []byte { return bytes.Replace(data, []byte(testRel), []byte("20990102_2"), 1) },
		"changes": func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"baseline":true`), []byte(`"baseline":false`), 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, noneDecision())
			item, output, err := f.process()
			if err != nil {
				t.Fatal(err)
			}
			files := map[string]string{}
			for _, file := range []string{findingsFile, identityFile, changesFile} {
				files[file] = string(f.artifactFile(output, file))
			}
			target := identityFile
			if name == "changes" {
				target = changesFile
			}
			files[target] = string(mutate([]byte(files[target])))
			other := f.publish(stage.EvidenceStage, item.InputHash, files)
			if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, other); err == nil || !strings.Contains(err.Error(), "bind") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestResolve_RefusesTamperedSidecarBytes(t *testing.T) {
	f := newFixture(t, noneDecision())
	item, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	appendTo(t, filepath.Join(f.env.Artifacts.Abs(output.Artifact.Path), identityFile), "{}\n")
	if _, err = f.stage.Resolve(context.Background(), f.env, f.run, item, *output.Artifact); err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("err = %v", err)
	}
}

func TestSidecarLines_StayWithinTheLineLimit(t *testing.T) {
	f := newFixture(t, noneDecision())
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{identityFile, changesFile} {
		scanner := bufio.NewScanner(bytes.NewReader(f.artifactFile(output, name)))
		for scanner.Scan() {
			var obj map[string]any
			if len(scanner.Bytes()) > lineLimit || json.Unmarshal(scanner.Bytes(), &obj) != nil {
				t.Fatalf("%s line invalid: %s", name, scanner.Bytes())
			}
		}
	}
}

func TestRecipe_NamesTheThreeFileSerializations(t *testing.T) {
	if serialization != "findings.json/2,identity.jsonl/1,changes.jsonl/1" {
		t.Fatalf("serialization %q", serialization)
	}
	if New(Config{}).Recipe(stage.Snapshot{}) != stage.RecipeHash(stage.EvidenceStage, "1", detector.Hash(), "[]", serialization) {
		t.Fatal("recipe does not cover the serialization")
	}
}
