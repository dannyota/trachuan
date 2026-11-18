package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/evidence"
)

func findingsFixture() evidence.File {
	return evidence.File{Schema: 1, Bindings: evidence.Bindings{Rel: "20990101_1"}, Findings: []evidence.Finding{
		{Key: "sha256:aa", Detector: "cross", Subject: evidence.Subject{Type: "document", Fields: []string{"vn:1"}}, Fingerprint: "sha256:bb", Semantics: 1, FirstRel: "20990101_1", Status: evidence.StatusOpen},
		{Key: "sha256:cc", Detector: "cross", Subject: evidence.Subject{Type: "document", Fields: []string{"vn:2"}}, Fingerprint: "sha256:dd", Semantics: 1, FirstRel: "20990101", Status: evidence.StatusAccepted, Carried: true},
	}, New: []string{"sha256:aa"}, Gone: []string{"sha256:ee"}, Carried: []string{"sha256:cc"}}
}

func execFindings(t *testing.T, args []string, load findingsLoader) (int, string, string) {
	t.Helper()
	var out, errout bytes.Buffer
	code := runFindingsWith(context.Background(), args, &out, &errout, load)
	return code, out.String(), errout.String()
}

func TestFindingsList_PrintsArtifactRows(t *testing.T) {
	load := func(_ context.Context, cc, rel string) (evidence.File, error) {
		if cc != "vn" || rel != "20990101_1" {
			t.Fatalf("loaded %s/%s", cc, rel)
		}
		return findingsFixture(), nil
	}
	code, out, errout := execFindings(t, []string{"list", "vn", "20990101_1"}, load)
	if code != 0 || errout != "" {
		t.Fatalf("code=%d err=%s", code, errout)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "KEY") || !strings.Contains(lines[1], "sha256:aa") || !strings.Contains(lines[2], "accepted") {
		t.Fatalf("output:\n%s", out)
	}
	if lines[3] != "findings 2 new 1 gone 1 carried 1 unmatched 0" {
		t.Fatalf("summary = %q", lines[3])
	}
}

func TestFindingsShow_PrintsOneFinding(t *testing.T) {
	load := func(context.Context, string, string) (evidence.File, error) { return findingsFixture(), nil }
	code, out, errout := execFindings(t, []string{"show", "vn", "20990101_1", "sha256:cc"}, load)
	if code != 0 || errout != "" {
		t.Fatalf("code=%d err=%s", code, errout)
	}
	var got evidence.Finding
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Key != "sha256:cc" || !got.Carried {
		t.Fatalf("finding=%+v err=%v", got, err)
	}
	if code, _, errout = execFindings(t, []string{"show", "vn", "20990101_1", "sha256:zz"}, load); code != 1 || !strings.Contains(errout, "not found") {
		t.Fatalf("missing key: code=%d err=%s", code, errout)
	}
}

func TestFindings_UsageAndLoadErrors(t *testing.T) {
	calls := 0
	load := func(context.Context, string, string) (evidence.File, error) {
		calls++
		return evidence.File{}, errors.New("no evidence")
	}
	for _, args := range [][]string{nil, {"list"}, {"list", "vn"}, {"list", "vn", "bad"}, {"list", "../", "20990101"}, {"show", "vn", "20990101"}, {"list", "vn", "20990101", "extra"}, {"delete", "vn", "20990101"}} {
		if code, _, errout := execFindings(t, args, load); code != 2 || !strings.Contains(errout, "usage") && !strings.Contains(errout, "invalid") {
			t.Errorf("args=%v code=%d err=%s", args, code, errout)
		}
	}
	if calls != 0 {
		t.Fatalf("loaded %d times for invalid arguments", calls)
	}
	if code, _, errout := execFindings(t, []string{"list", "vn", "20990101"}, load); code != 1 || !strings.Contains(errout, "no evidence") {
		t.Fatalf("code=%d err=%s", code, errout)
	}
}

func TestRun_RegistersFindings(t *testing.T) {
	var out, errout bytes.Buffer
	if code := Run(context.Background(), []string{"findings"}, &out, &errout); code != 2 || !strings.Contains(errout.String(), "corpus findings") {
		t.Fatalf("code=%d err=%s", code, errout.String())
	}
	errout.Reset()
	if code := Run(context.Background(), []string{"unknown"}, &out, &errout); code != 2 || !strings.Contains(errout.String(), "findings") {
		t.Fatalf("usage omits findings: %s", errout.String())
	}
}
