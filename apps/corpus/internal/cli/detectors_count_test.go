package cli

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage/evidence"
)

func countWith(t *testing.T, load detectorCounter, args ...string) (int, string, string) {
	t.Helper()
	var out, errout bytes.Buffer
	code := runDetectorsCount(context.Background(), args, &out, &errout, load)
	return code, out.String(), errout.String()
}

func TestDetectorsCount_PrintsCountsAndClasses(t *testing.T) {
	load := func(_ context.Context, cc, rel string) ([]evidence.Count, error) {
		if cc != "vn" || rel != "20990102_1" {
			t.Fatalf("release %s/%s", cc, rel)
		}
		return []evidence.Count{
			{Detector: "raw_field_not_normalized", Severity: detector.Info, Findings: 9, Observed: 100},
			{Detector: "relation_kind_disagreement", Severity: detector.Review, Findings: 4, Excluded: 6, Observed: 50},
			{Detector: "unmatched_operative_claim", Severity: detector.Review, Findings: 3, Observed: 100, NotObservable: 4, Classes: map[string]int{"review": 2, "info": 1}},
		}, nil
	}
	code, out, errout := countWith(t, load, "vn", "20990102_1")
	if code != 0 || errout != "" {
		t.Fatalf("code=%d err=%q", code, errout)
	}
	if !regexp.MustCompile(`relation_kind_disagreement\s+review\s+4\s+50\s+0\s+6`).MatchString(out) || !regexp.MustCompile(`raw_field_not_normalized\s+info\s+9\s+100\s+0\s+0`).MatchString(out) {
		t.Errorf("excluded column is wrong:\n%s", out)
	}
	for _, want := range []string{"raw_field_not_normalized", "unmatched_operative_claim", "  review 2", "  info 1", "relation_kind_disagreement", "EXCLUDED"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestDetectorsCount_NamesDetectorsNotRun(t *testing.T) {
	load := func(context.Context, string, string) ([]evidence.Count, error) {
		return []evidence.Count{{Detector: "waived_document_without_relations", NotRun: "extract artifacts are not available"}}, nil
	}
	code, out, _ := countWith(t, load, "vn", "20990102_1")
	if code != 1 || !strings.Contains(out, "not run: extract artifacts are not available") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestDetectorsCount_RejectsBadArguments(t *testing.T) {
	load := func(context.Context, string, string) ([]evidence.Count, error) { t.Fatal("loaded"); return nil, nil }
	for _, args := range [][]string{{}, {"vn"}, {"vn", "20990102_1", "extra"}, {"vn", "latest"}, {"VN", "20990102_1"}} {
		if code, _, _ := countWith(t, load, args...); code != 2 {
			t.Errorf("%v: code %d", args, code)
		}
	}
}

func TestDetectorsCount_ReportsLoadFailure(t *testing.T) {
	load := func(context.Context, string, string) ([]evidence.Count, error) {
		return nil, errors.New("release database unavailable")
	}
	code, _, errout := countWith(t, load, "vn", "20990102_1")
	if code != 1 || !strings.Contains(errout, "release database unavailable") {
		t.Fatalf("code=%d err=%q", code, errout)
	}
}

func TestDetectorsCommand_RoutesCountToTheLoader(t *testing.T) {
	called := ""
	load := func(_ context.Context, cc, rel string) ([]evidence.Count, error) {
		called = cc + "/" + rel
		return []evidence.Count{{Detector: "raw_field_not_normalized", Findings: 7}}, nil
	}
	var out, errout bytes.Buffer
	code := runDetectorsWith(context.Background(), []string{"count", "vn", "20990102_1"}, &out, &errout, load)
	if code != 0 || called != "vn/20990102_1" || !strings.Contains(out.String(), "raw_field_not_normalized") {
		t.Fatalf("code=%d called=%q out=%q err=%q", code, called, out.String(), errout.String())
	}
	if code = runDetectorsWith(context.Background(), []string{"list"}, &out, &errout, load); code != 0 || called != "vn/20990102_1" {
		t.Fatalf("list code=%d", code)
	}
}

func TestLoadDetectorCounts_FailsClosedWithoutDatabases(t *testing.T) {
	for _, name := range []string{"CORPUS_DATABASE_URL", "CORPUS_BUILDER_DATABASE_URL"} {
		t.Setenv(name, "")
	}
	if _, err := loadDetectorCounts(context.Background(), "vn", "latest"); err == nil {
		t.Fatal("invalid release accepted")
	}
	if _, err := loadDetectorCounts(context.Background(), "vn", "20990102_1"); err == nil {
		t.Fatal("no database configuration accepted")
	}
}
