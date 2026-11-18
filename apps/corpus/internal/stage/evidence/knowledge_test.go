package evidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

const otherHash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func (f *fixture) buildWithKnowledge(body string) {
	f.t.Helper()
	f.build = f.publish("build", bareSHA256([]byte(body)), map[string]string{"build.json": "{}", knowledgeFile: body})
	f.report["build_artifact_sha256"] = f.build.SHA256
	f.sync()
}

func decisionEntry(d ReviewDecision) string {
	return fmt.Sprintf(`- id: %q
  kind: "review_decision"
  target: { finding: %q }
  claim: { reason_code: "false_positive", reason: "Reviewed and needs no change." }
  depends: { fingerprint: %q, semantics: %d, first_rel: %q }
  reviewed: { date: "2099-01-01", by: "primary", method: "human" }
`, d.ID, d.Key, d.Fingerprint, d.Semantics, d.FirstRel)
}

func firedDecision(t *testing.T, subject Fired, firstRel string) ReviewDecision {
	t.Helper()
	key, err := findingKey("cross", subject)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := hashValue(subject.FingerprintInputs)
	if err != nil {
		t.Fatal(err)
	}
	return ReviewDecision{ID: "decision-a", Key: key, Fingerprint: fingerprint, Semantics: 1, FirstRel: firstRel}
}

var oneFieldDiffers = []struct {
	name   string
	mutate func(*ReviewDecision)
}{
	{"other key", func(d *ReviewDecision) { d.Key = otherHash }},
	{"other fingerprint", func(d *ReviewDecision) { d.Fingerprint = otherHash }},
	{"other semantics", func(d *ReviewDecision) { d.Semantics = 2 }},
	{"other first_rel", func(d *ReviewDecision) { d.FirstRel = "20990103_1" }},
}

func assertUnmatched(t *testing.T, file File, want ...ReviewDecision) {
	t.Helper()
	if len(file.Unmatched) != len(want) || (len(want) > 0 && !reflect.DeepEqual(file.Unmatched, want)) {
		t.Fatalf("unmatched = %+v, want %+v", file.Unmatched, want)
	}
}

func TestProcess_DecisionAcceptsTheFinding(t *testing.T) {
	subject := fired("vn:1", "", "x")
	exact := firedDecision(t, subject, testRel)
	f := newFixture(t, noneDecision())
	f.buildWithKnowledge(decisionEntry(exact))
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross", observation: Observation{Findings: []Fired{subject}, ObservedCompletely: always}}}
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	file := f.findings(output)
	if got := only(t, file); got.Status != StatusAccepted || got.Carried {
		t.Fatalf("finding = %+v", got)
	}
	assertUnmatched(t, file)
}

func TestProcess_DecisionDifferingInOneFieldIsUnmatched(t *testing.T) {
	subject := fired("vn:1", "", "x")
	for _, tc := range oneFieldDiffers {
		t.Run(tc.name, func(t *testing.T) {
			decision := firedDecision(t, subject, testRel)
			tc.mutate(&decision)
			f := newFixture(t, noneDecision())
			f.buildWithKnowledge(decisionEntry(decision))
			f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross", observation: Observation{Findings: []Fired{subject}, ObservedCompletely: always}}}
			_, output, err := f.process()
			if err != nil {
				t.Fatal(err)
			}
			file := f.findings(output)
			if got := only(t, file); got.Status != StatusOpen {
				t.Fatalf("finding = %+v", got)
			}
			assertUnmatched(t, file, decision)
		})
	}
}

func carriedFixture(t *testing.T, subject Fired, decision ReviewDecision, observed func(Subject) bool) *fixture {
	t.Helper()
	frozen := strings.Repeat("a", 64)
	exact := firedDecision(t, subject, previousRel)
	prior := Finding{Key: exact.Key, Detector: "cross", Subject: subject.Subject, Fingerprint: exact.Fingerprint, Semantics: 1, FirstRel: previousRel, Status: StatusOpen}
	f := newFixture(t, stage.PreviousReport{SHA256: frozen})
	f.buildWithKnowledge(decisionEntry(decision))
	f.comparedWith(frozen)
	f.predecessorRun(true, "evaluated", predecessorFile(frozen, prior))
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross", observation: Observation{ObservedCompletely: observed}}}
	return f
}

func TestProcess_DecisionJudgesCarriedFinding(t *testing.T) {
	subject := fired("vn:1", "", "x")
	exact := firedDecision(t, subject, previousRel)
	t.Run("exact", func(t *testing.T) {
		f := carriedFixture(t, subject, exact, never)
		_, output, err := f.process()
		if err != nil {
			t.Fatal(err)
		}
		file := f.findings(output)
		if got := only(t, file); got.Status != StatusAccepted || !got.Carried {
			t.Fatalf("finding = %+v", got)
		}
		assertUnmatched(t, file)
	})
	for _, tc := range oneFieldDiffers {
		t.Run(tc.name, func(t *testing.T) {
			decision := exact
			tc.mutate(&decision)
			f := carriedFixture(t, subject, decision, never)
			_, output, err := f.process()
			if err != nil {
				t.Fatal(err)
			}
			file := f.findings(output)
			if got := only(t, file); got.Status != StatusOpen || !got.Carried {
				t.Fatalf("finding = %+v", got)
			}
			assertUnmatched(t, file, decision)
		})
	}
}

func TestProcess_DecisionOnGoneKeyIsUnmatched(t *testing.T) {
	subject := fired("vn:1", "", "x")
	exact := firedDecision(t, subject, previousRel)
	f := carriedFixture(t, subject, exact, always)
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	file := f.findings(output)
	if len(file.Findings) != 0 || !reflect.DeepEqual(file.Gone, []string{exact.Key}) {
		t.Fatalf("file = %+v", file)
	}
	assertUnmatched(t, file, exact)
}

func TestProcess_KnowledgeWithoutEntriesHasNoDecisions(t *testing.T) {
	for name, body := range map[string]string{"empty": "", "comments only": "# nothing reviewed\n", "empty list": "[]\n"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, noneDecision())
			f.buildWithKnowledge(body)
			_, output, err := f.process()
			if err != nil {
				t.Fatal(err)
			}
			assertUnmatched(t, f.findings(output))
		})
	}
}

func TestProcess_UnparsableKnowledgeIsDetectorIncomplete(t *testing.T) {
	valid := ReviewDecision{ID: "decision-b", Key: otherHash, Fingerprint: otherHash, Semantics: 1, FirstRel: testRel}
	earlier := valid
	earlier.ID = "decision-a"
	for name, body := range map[string]string{
		"not a list":       "entries: []\n",
		"not yaml":         "- [\n",
		"two documents":    "[]\n---\n[]\n",
		"unknown field":    decisionEntry(valid) + "  extra: 1\n",
		"entries unsorted": decisionEntry(valid) + decisionEntry(earlier),
		"bad finding hash": strings.ReplaceAll(decisionEntry(valid), otherHash, "sha256:zz"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, noneDecision())
			f.buildWithKnowledge(body)
			_, output, err := f.process()
			if err == nil || !strings.HasPrefix(err.Error(), "detector_incomplete: knowledge: invalid knowledge: ") || output.Artifact != nil {
				t.Fatalf("output=%+v err=%v", output, err)
			}
			assertPermanent(t, err)
		})
	}
}

func TestInputHash_SeparatesAbsentEmptyAndChangedKnowledge(t *testing.T) {
	hashes := map[string]bool{}
	for _, knowledge := range [][]byte{nil, {}, []byte("[]\n")} {
		hashes[frozenInputs{knowledge: knowledge}.inputHash()] = true
	}
	if len(hashes) != 3 {
		t.Fatalf("hashes = %v", hashes)
	}
}

func replaceFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func symlinkFile(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func hideFromManifest(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "MANIFEST.sha256")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for line := range strings.Lines(string(data)) {
		if !strings.HasSuffix(line, "  "+knowledgeFile+"\n") {
			kept = append(kept, line)
		}
	}
	replaceFile(t, path, strings.Join(kept, ""))
	removeFile(t, filepath.Join(dir, knowledgeFile))
}

func TestKnowledgePresence_ComesFromTheBuildManifest(t *testing.T) {
	const (
		listedBody = "[]\n"
		unverified = "knowledge file failed verification"
		unlisted   = "knowledge file its manifest does not list"
		tampered   = "build artifact failed verification"
	)
	for _, tc := range []struct {
		name   string
		listed bool
		damage func(t *testing.T, dir, path string)
		want   string
	}{
		{"listed file missing", true, func(t *testing.T, _, path string) { removeFile(t, path) }, unverified},
		{"listed file changed", true, func(t *testing.T, _, path string) { replaceFile(t, path, "# changed\n") }, unverified},
		{"listed file is a symlink", true, func(t *testing.T, _, path string) {
			outside := filepath.Join(t.TempDir(), knowledgeFile)
			replaceFile(t, outside, listedBody)
			removeFile(t, path)
			symlinkFile(t, outside, path)
		}, unverified},
		{"manifest rewritten to hide the file", true, func(t *testing.T, dir, _ string) { hideFromManifest(t, dir) }, tampered},
		{"unlisted file", false, func(t *testing.T, _, path string) { replaceFile(t, path, listedBody) }, unlisted},
		{"unlisted empty file", false, func(t *testing.T, _, path string) { replaceFile(t, path, "") }, unlisted},
		{"unlisted dangling symlink", false, func(t *testing.T, dir, path string) { symlinkFile(t, filepath.Join(dir, "absent"), path) }, unlisted},
		{"unlisted directory", false, func(t *testing.T, _, path string) {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}, unlisted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newFixture(t, noneDecision())
			if tc.listed {
				f.buildWithKnowledge(listedBody)
			}
			item, output, err := f.process()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.stage.Resolve(ctx, f.env, f.run, item, *output.Artifact); err != nil {
				t.Fatal(err)
			}
			dir := f.env.Artifacts.Abs(f.build.Path)
			if err = os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			tc.damage(t, dir, filepath.Join(dir, knowledgeFile))

			items, err := f.stage.Plan(ctx, f.env, f.run)
			if err != nil {
				t.Fatal(err)
			}
			refused, err := f.stage.Process(ctx, f.env, f.run, items[0])
			if err == nil || !strings.Contains(err.Error(), tc.want) || refused.Artifact != nil {
				t.Fatalf("process output=%+v err=%v", refused, err)
			}
			assertPermanent(t, err)
			if want := stage.RecipeHash("evidence-input-refused", "1", err.Error()); items[0].InputHash != want {
				t.Fatalf("plan hash = %s, want the refused hash for %q", items[0].InputHash, err)
			}
			if _, err = f.stage.Resolve(ctx, f.env, f.run, item, *output.Artifact); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("resolve err = %v", err)
			}
		})
	}
}
