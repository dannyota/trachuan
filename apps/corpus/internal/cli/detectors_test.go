package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

func execDetectors(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errout bytes.Buffer
	code := Run(context.Background(), append([]string{"detectors"}, args...), &out, &errout)
	return code, out.String(), errout.String()
}

func TestDetectorsList_PrintsStableTable(t *testing.T) {
	code, first, errout := execDetectors(t, "list")
	if code != 0 || errout != "" {
		t.Fatalf("code=%d err=%q", code, errout)
	}
	_, second, _ := execDetectors(t, "list")
	if first != second {
		t.Fatal("table differs between runs")
	}
	lines := strings.Split(strings.TrimSpace(first), "\n")
	if !strings.HasPrefix(lines[0], "ID") || !strings.Contains(lines[0], "SEVERITY") {
		t.Fatalf("header = %q", lines[0])
	}
	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, "baseline_served") && strings.Contains(line, "review") {
			found = true
		}
	}
	if !found {
		t.Fatalf("baseline_served row missing:\n%s", first)
	}
	if want := "registry hash " + detector.Hash(); lines[len(lines)-1] != want {
		t.Fatalf("last line = %q, want %q", lines[len(lines)-1], want)
	}
	if len(lines) != len(detector.All())+2 {
		t.Fatalf("%d lines for %d detectors", len(lines), len(detector.All()))
	}
}

func TestDetectorsList_JSONCarriesCanonicalFormAndHash(t *testing.T) {
	code, out, errout := execDetectors(t, "list", "--json")
	if code != 0 || errout != "" {
		t.Fatalf("code=%d err=%q", code, errout)
	}
	var got struct {
		Hash     string          `json:"hash"`
		Registry json.RawMessage `json:"registry"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Hash != detector.Hash() || !bytes.Equal(got.Registry, detector.Canonical()) {
		t.Fatalf("json output differs from the registry: hash=%s", got.Hash)
	}
}

func TestDetectors_UsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"show"}, {"list", "--yaml"}, {"list", "extra"}} {
		code, out, errout := execDetectors(t, args...)
		if code != 2 || out != "" || !strings.Contains(errout, "usage: corpus detectors list") {
			t.Errorf("args %v: code=%d out=%q err=%q", args, code, out, errout)
		}
	}
}
