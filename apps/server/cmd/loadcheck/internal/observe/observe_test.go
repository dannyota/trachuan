package observe

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRestarts_Identity(t *testing.T) {
	tracker := newTracker()
	if tracker.observe("web", []process{{PID: 1, Start: "Mon Dec 30 12:00:00 2025"}}) {
		t.Fatal("initial process restarted")
	}
	if tracker.observe("web", []process{{PID: 1, Start: "Mon Dec 30 12:00:00 2025"}}) {
		t.Fatal("unchanged process restarted")
	}
	if !tracker.observe("web", []process{{PID: 1, Start: "Thu Jan 1 12:00:00 2026"}}) {
		t.Fatal("start change missed")
	}
	if !tracker.observe("web", []process{{PID: 2, Start: "Thu Jan 1 12:00:00 2026"}}) {
		t.Fatal("PID change missed")
	}
}
func TestRenders_WindowAndValidation(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Second)
	input := `{"msg":"summary render","time":"2026-01-01T00:00:02.000Z","path":"/vn/documents/vn:fixture:1/summary"}` + "\n" + `{"msg":"summary render","time":"2026-01-01T00:00:04.000Z","path":"/vn/documents/vn:fixture:1/summary"}` + "\n" + `{"msg":"other"}`
	counts, err := renders(strings.NewReader(input), start, end)
	if err != nil || counts["vn:fixture:1"] != 2 {
		t.Fatalf("%v %v", counts, err)
	}
	if _, err = renders(strings.NewReader(`{"msg":"summary render","path":"/x"}`), start, end); err == nil {
		t.Fatal("missing timestamp accepted")
	}
	if _, err = renders(strings.NewReader(`{"msg":"summary render","time":"2026-01-01T07:00:02+07:00","path":"/vn/documents/vn:fixture:1/summary"}`), start, end); err == nil {
		t.Fatal("non-UTC timestamp accepted")
	}
}
func TestBuckets_CombinedP99(t *testing.T) {
	logs := `{"msg":"reader pool wait","time":"2026-01-01T00:00:01Z","bucket_bounds_ms":[1,50],"bucket_counts":[99,1,0],"count":100}` + "\n" + `{"msg":"reader pool wait","time":"2026-01-01T00:00:02Z","bucket_bounds_ms":[1,50],"bucket_counts":[0,0,1],"count":1}`
	got, err := poolP99(strings.NewReader(logs), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC))
	if err != nil || got != 50 {
		t.Fatalf("p99 %v err %v", got, err)
	}
}
func TestLatency_Relative(t *testing.T) {
	if !latencyGate(125, 200, 100, 1000, 2000) || latencyGate(126, 200, 100, 1000, 2000) {
		t.Fatal("relative gate")
	}
}

func TestProcesses_AggregateAndTrackOldest(t *testing.T) {
	lines := "100 Mon Dec 30 12:00:00 2025 100 1.5 /usr/bin/postgres\n" +
		"101 Thu Jan  1 12:00:00 2026 200 2.5 postgres: writer\n" +
		"102 Fri Jan  2 12:00:00 2026 300 3.5 postgres: checkpointer\n" +
		"103 Fri Jan  2 12:00:00 2026 900 9.5 postgresql-client\n"
	patterns := map[string]*regexp.Regexp{"postgres": regexp.MustCompile(`(^|/)postgres(:|$)`)}
	groups, _, err := parseProcesses(strings.NewReader(lines), patterns, 0)
	if err != nil {
		t.Fatal(err)
	}
	tr := newTracker()
	sample := summarizeProcesses(tr, "postgres", groups["postgres"])
	if sample.Count != 3 || sample.RSS != 600*1024 || sample.CPU != 7.5 || sample.Restart {
		t.Fatalf("sample %+v", sample)
	}
	data, err := json.Marshal(sample)
	if err != nil || !strings.Contains(string(data), `"count":3`) {
		t.Fatalf("count missing from sample JSON: %v", err)
	}
	if summarizeProcesses(tr, "postgres", groups["postgres"][:2]).Restart {
		t.Fatal("worker exit counted as restart")
	}
	added := append(append([]process(nil), groups["postgres"]...), process{PID: 104, Start: "Sat Jan 3 12:00:00 2026"})
	if summarizeProcesses(tr, "postgres", added).Restart {
		t.Fatal("worker start counted as restart")
	}
	if !summarizeProcesses(tr, "postgres", groups["postgres"][1:]).Restart {
		t.Fatal("oldest process change missed")
	}
}

func TestProcesses_RejectInvalidStartTime(t *testing.T) {
	lines := "100 Bad Xxx 01 00:00:00 2026 100 1.5 postgres\n"
	_, _, err := parseProcesses(strings.NewReader(lines), map[string]*regexp.Regexp{"postgres": regexp.MustCompile(`^postgres$`)}, 0)
	if err == nil {
		t.Fatal("invalid process start accepted")
	}
}

func TestProcesses_RecordsUnmatchedName(t *testing.T) {
	groups, _, err := parseProcesses(strings.NewReader(""), map[string]*regexp.Regexp{"postgres": regexp.MustCompile(`postgres`)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := groups["postgres"]; !ok {
		t.Fatal("missing zero-count process group")
	}
}

func TestSample_WithoutPostgres(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "samples.jsonl")
	ps := filepath.Join(dir, "ps")
	if err := os.WriteFile(ps, []byte("#!/bin/sh\nprintf '%s\\n' '123 Thu Jan  1 12:00:00 2026 100 1.5 fixture'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := exec.Command("env", "LC_ALL=C", "ps").Run(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := Sample(ctx, []string{"--out", path, "--proc", "self=fixture"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if value, ok := got["connections"]; ok && string(value) != "null" {
		t.Fatalf("connections = %s, want absent or null", value)
	}
	for _, name := range []string{"time", "host_cpu_percent", "processes"} {
		if _, ok := got[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	var processes map[string]processSample
	if err := json.Unmarshal(got["processes"], &processes); err != nil {
		t.Fatal(err)
	}
	if p := processes["self"]; p.Count != 1 || p.RSS != 102400 || p.CPU != 1.5 || len(p.Processes) != 1 || p.Restart {
		t.Fatalf("process sample = %+v", p)
	}
}
