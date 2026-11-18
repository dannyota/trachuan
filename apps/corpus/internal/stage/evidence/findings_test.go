package evidence

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

func docSubject(key string) Subject { return Subject{Type: "document", Fields: []string{key}} }

func fired(key, discriminator string, inputs ...string) Fired {
	return Fired{Subject: docSubject(key), Discriminator: discriminator, FingerprintInputs: inputs}
}

func run(id string, semantics int, observed func(Subject) bool, findings ...Fired) detectorRun {
	return detectorRun{spec: detector.Detector{ID: id, Version: 1, Semantics: semantics}, observation: Observation{Findings: findings, Observed: 10, NotObservable: 2, ObservedCompletely: observed}}
}

func always(Subject) bool { return true }
func never(Subject) bool  { return false }

func mustCompute(t *testing.T, in computeInput) File {
	t.Helper()
	if in.Rel == "" {
		in.Rel = "20990102_1"
	}
	file, err := compute(in)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func only(t *testing.T, file File) Finding {
	t.Helper()
	if len(file.Findings) != 1 {
		t.Fatalf("findings = %+v", file.Findings)
	}
	return file.Findings[0]
}

func TestCompute_EmptyInputWritesEmptyLists(t *testing.T) {
	file := mustCompute(t, computeInput{})
	data, err := file.encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema": 1`, `"detectors": {}`, `"findings": []`, `"new": []`, `"gone": []`, `"carried": []`, `"unmatched": []`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %s in %s", want, data)
		}
	}
}

func TestCompute_KeySeparatesDiscriminatorsAndHashForm(t *testing.T) {
	file := mustCompute(t, computeInput{Runs: []detectorRun{run("d", 1, always, fired("vn:1", "first", "x"), fired("vn:1", "second", "x"))}})
	if len(file.Findings) != 2 || file.Findings[0].Key == file.Findings[1].Key {
		t.Fatalf("findings = %+v", file.Findings)
	}
	for _, f := range file.Findings {
		if !strings.HasPrefix(f.Key, "sha256:") || len(f.Key) != 71 || !strings.HasPrefix(f.Fingerprint, "sha256:") || len(f.Fingerprint) != 71 {
			t.Fatalf("hash form: %+v", f)
		}
		if f.FirstRel != "20990102_1" || f.Status != StatusOpen || f.Carried {
			t.Fatalf("finding = %+v", f)
		}
	}
	if len(file.New) != 2 || !sortedStrings(file.New) {
		t.Fatalf("new = %v", file.New)
	}
	if got := file.Detectors["d"]; got != (DetectorCounts{Complete: true, Observed: 10, NotObservable: 2}) {
		t.Fatalf("counts = %+v", got)
	}
}

func sortedStrings(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] > values[i] {
			return false
		}
	}
	return true
}

func previousFrom(t *testing.T, rel string, runs ...detectorRun) *File {
	t.Helper()
	file := mustCompute(t, computeInput{Rel: rel, Runs: runs})
	return &file
}

func TestCompute_FirstRelInheritedOnlyFromSameFingerprintAndSemantics(t *testing.T) {
	previous := previousFrom(t, "20990101_1", run("d", 1, always, fired("vn:1", "", "x")))
	for _, tc := range []struct {
		name  string
		now   detectorRun
		first string
		isNew bool
	}{
		{"unchanged", run("d", 1, always, fired("vn:1", "", "x")), "20990101_1", false},
		{"changed fingerprint", run("d", 1, always, fired("vn:1", "", "y")), "20990102_1", true},
		{"new semantics", run("d", 2, always, fired("vn:1", "", "x")), "20990102_1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := mustCompute(t, computeInput{Previous: previous, Runs: []detectorRun{tc.now}})
			f := only(t, file)
			if f.FirstRel != tc.first || (len(file.New) == 1) != tc.isNew {
				t.Fatalf("first=%s new=%v", f.FirstRel, file.New)
			}
		})
	}
}

func TestCompute_CarriedFindingKeepsOccurrenceThenReturnsAsSame(t *testing.T) {
	original := previousFrom(t, "20990101_1", run("d", 1, always, fired("vn:1", "", "x")))
	carried := mustCompute(t, computeInput{Rel: "20990102_1", Previous: original, Runs: []detectorRun{run("d", 1, never)}})
	f := only(t, carried)
	if !f.Carried || f.FirstRel != "20990101_1" || f.Fingerprint != original.Findings[0].Fingerprint || f.Status != StatusOpen {
		t.Fatalf("carried = %+v", f)
	}
	if !reflect.DeepEqual(carried.Carried, []string{f.Key}) || len(carried.Gone) != 0 || len(carried.New) != 0 {
		t.Fatalf("lists = %+v", carried)
	}
	back := mustCompute(t, computeInput{Rel: "20990103_1", Previous: &carried, Runs: []detectorRun{run("d", 1, always, fired("vn:1", "", "x"))}})
	if got := only(t, back); got.FirstRel != "20990101_1" || got.Carried {
		t.Fatalf("returned = %+v", got)
	}
}

func TestCompute_CarriedFindingDropsPredecessorAcceptance(t *testing.T) {
	original := previousFrom(t, "20990101_1", run("d", 1, always, fired("vn:1", "", "x")))
	original.Findings[0].Status = StatusAccepted
	carried := mustCompute(t, computeInput{Rel: "20990102_1", Previous: original, Runs: []detectorRun{run("d", 1, never)}})
	if f := only(t, carried); !f.Carried || f.Status != StatusOpen {
		t.Fatalf("carried = %+v", f)
	}
}

func TestCompute_GoneKeyIsARecordAndEndsTheOccurrence(t *testing.T) {
	original := previousFrom(t, "20990101_1", run("d", 1, always, fired("vn:1", "", "x")))
	gone := mustCompute(t, computeInput{Rel: "20990102_1", Previous: original, Runs: []detectorRun{run("d", 1, always)}})
	if len(gone.Findings) != 0 || !reflect.DeepEqual(gone.Gone, []string{original.Findings[0].Key}) {
		t.Fatalf("gone = %+v", gone)
	}
	back := mustCompute(t, computeInput{Rel: "20990103_1", Previous: &gone, Runs: []detectorRun{run("d", 1, always, fired("vn:1", "", "x"))}})
	if got := only(t, back); got.FirstRel != "20990103_1" {
		t.Fatalf("returned finding kept its old occurrence: %+v", got)
	}
}

func TestCompute_UnobservedPreviousFindingIsCarried(t *testing.T) {
	original := previousFrom(t, "20990101_1", run("d", 1, always, fired("vn:1", "", "x")))
	for name, runs := range map[string][]detectorRun{
		"detector not run":  nil,
		"no observation fn": {run("d", 1, nil)},
		"subject not seen":  {run("d", 1, never)},
	} {
		t.Run(name, func(t *testing.T) {
			file := mustCompute(t, computeInput{Previous: original, Runs: runs})
			if f := only(t, file); !f.Carried || len(file.Gone) != 0 {
				t.Fatalf("file = %+v", file)
			}
		})
	}
}

func TestCompute_DecisionAcceptsOnlyExactFinding(t *testing.T) {
	base := only(t, mustCompute(t, computeInput{Runs: []detectorRun{run("d", 1, always, fired("vn:1", "", "x"))}}))
	exact := ReviewDecision{ID: "r1", Key: base.Key, Fingerprint: base.Fingerprint, Semantics: 1, FirstRel: base.FirstRel}
	for _, tc := range []struct {
		name     string
		mutate   func(*ReviewDecision)
		accepted bool
	}{
		{"exact", func(*ReviewDecision) {}, true},
		{"other fingerprint", func(d *ReviewDecision) { d.Fingerprint = "sha256:" + strings.Repeat("0", 64) }, false},
		{"other semantics", func(d *ReviewDecision) { d.Semantics = 9 }, false},
		{"other occurrence", func(d *ReviewDecision) { d.FirstRel = "20990101_1" }, false},
		{"other key", func(d *ReviewDecision) { d.Key = "sha256:" + strings.Repeat("1", 64) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := exact
			tc.mutate(&decision)
			file := mustCompute(t, computeInput{Decisions: []ReviewDecision{decision}, Runs: []detectorRun{run("d", 1, always, fired("vn:1", "", "x"))}})
			f := only(t, file)
			if (f.Status == StatusAccepted) != tc.accepted {
				t.Fatalf("status = %s", f.Status)
			}
			if tc.accepted == (len(file.Unmatched) == 1) {
				t.Fatalf("unmatched = %+v", file.Unmatched)
			}
		})
	}
}

func TestCompute_DecisionJudgesCarriedFindingAndIgnoresGone(t *testing.T) {
	original := previousFrom(t, "20990101_1", run("d", 1, always, fired("vn:1", "", "x")))
	prior := original.Findings[0]
	decision := ReviewDecision{ID: "r1", Key: prior.Key, Fingerprint: prior.Fingerprint, Semantics: 1, FirstRel: prior.FirstRel}
	carried := mustCompute(t, computeInput{Previous: original, Decisions: []ReviewDecision{decision}, Runs: []detectorRun{run("d", 1, never)}})
	if f := only(t, carried); f.Status != StatusAccepted || len(carried.Unmatched) != 0 {
		t.Fatalf("carried = %+v", carried)
	}
	gone := mustCompute(t, computeInput{Previous: original, Decisions: []ReviewDecision{decision}, Runs: []detectorRun{run("d", 1, always)}})
	if len(gone.Unmatched) != 1 || gone.Unmatched[0].ID != "r1" {
		t.Fatalf("gone = %+v", gone)
	}
}

func TestCompute_RefusesBrokenDetectorOutput(t *testing.T) {
	for name, finding := range map[string][]Fired{
		"duplicate key": {fired("vn:1", "", "x"), fired("vn:1", "", "y")},
		"unknown type":  {{Subject: Subject{Type: "article", Fields: []string{"x"}}}},
		"wrong arity":   {{Subject: Subject{Type: "provision", Fields: []string{"x"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := compute(computeInput{Rel: "20990102_1", Runs: []detectorRun{run("broken", 1, always, finding...)}})
			var incomplete *IncompleteError
			if !errors.As(err, &incomplete) || incomplete.Detector != "broken" || !strings.HasPrefix(err.Error(), "detector_incomplete: broken") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestCompute_IsPureAndOrderIndependent(t *testing.T) {
	first := mustCompute(t, computeInput{Runs: []detectorRun{run("b", 1, always, fired("vn:2", "", "x")), run("a", 1, always, fired("vn:1", "", "x"))}})
	second := mustCompute(t, computeInput{Runs: []detectorRun{run("a", 1, always, fired("vn:1", "", "x")), run("b", 1, always, fired("vn:2", "", "x"))}})
	one, err := first.encode()
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(one) != string(two) {
		t.Fatalf("output depends on detector order:\n%s\n%s", one, two)
	}
}
