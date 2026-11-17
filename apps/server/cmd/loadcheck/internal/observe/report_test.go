package observe

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

func TestReport_MissingBaselineIsUnavailable(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	requests := []loadrun.Record{{Phase: "C", Stream: "search", Status: 200, Start: at, DurationMS: 10}, {Phase: "C", Stream: "reader", Status: 200, Start: at, DurationMS: 10}}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(requests, loadrun.Report{}, nil, strings.NewReader(""), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == "C search" {
			if g.Result != "unavailable" {
				t.Fatalf("result %s", g.Result)
			}
			return
		}
	}
	t.Fatal("missing gate")
}

func TestReport_CacheHitsUsePhaseWindow(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []loadrun.Record{{Phase: "A", Stream: "search", Status: 200, Start: at, DurationMS: 10}, {Phase: "A", Stream: "reader", Status: 200, Start: at, DurationMS: 10}}
	var web strings.Builder
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("vn:fixture:%d", i)
		records = append(records, loadrun.Record{Phase: "C", Stream: "summary", Key: key, Pass: 1, Status: 200, Start: at.Add(time.Second), DurationMS: 10}, loadrun.Record{Phase: "C", Stream: "summary", Key: key, Pass: 2, Status: 200, Start: at.Add(2 * time.Second), DurationMS: 10})
		if i < 6 {
			_, _ = fmt.Fprintf(&web, "{\"msg\":\"summary render\",\"time\":\"2026-01-01T00:00:01.500Z\",\"path\":\"/vn/documents/%s/summary\"}\n{\"msg\":\"summary render\",\"time\":\"2026-01-01T00:00:01.600Z\",\"path\":\"/vn/documents/%s/summary\"}\n", key, key)
		}
	}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:02Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, nil, strings.NewReader(web.String()), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == "C hit rate" {
			if g.Result != "fail" || !strings.Contains(g.Value, "494/500") {
				t.Fatalf("gate %+v", g)
			}
			return
		}
	}
	t.Fatal("missing hit gate")
}

func TestReport_GapDuringDLeavesPeaksUnavailable(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []loadrun.Record{
		{Phase: "A", Stream: "search", Status: 200, Start: start, DurationMS: 10000},
		{Phase: "A", Stream: "reader", Status: 200, Start: start, DurationMS: 10000},
		{Phase: "D", Stream: "search", Status: 200, Start: start.Add(20 * time.Second), DurationMS: 20000},
		{Phase: "D", Stream: "reader", Status: 200, Start: start.Add(20 * time.Second), DurationMS: 20000},
	}
	cpu := 10.0
	processes := map[string]processSample{}
	for _, name := range []string{"web", "server", "postgres"} {
		processes[name] = processSample{Processes: []process{{PID: 1}}}
	}
	var samples []sample
	for _, seconds := range []int{0, 4, 8, 10, 20, 24, 36, 40} {
		samples = append(samples, sample{Time: start.Add(time.Duration(seconds) * time.Second), HostCPU: &cpu, Processes: processes})
	}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:30Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, samples, strings.NewReader(""), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"PostgreSQL connections", "Host CPU", "web RSS", "server RSS", "Restarts"} {
		found := false
		for _, g := range gates {
			if g.Name == name {
				found = true
				if g.Result != "unavailable" {
					t.Errorf("%s: %s", name, g.Result)
				}
			}
		}
		if !found {
			t.Errorf("missing %s", name)
		}
	}
}

func TestCoverage_RequiresEveryProcessAndEdges(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []loadrun.Record{{Phase: "D", Start: start, DurationMS: 12000}}
	sampleAt := func(seconds int, names ...string) sample {
		ps := map[string]processSample{}
		for _, name := range names {
			ps[name] = processSample{Processes: []process{{PID: 1}}}
		}
		return sample{Time: start.Add(time.Duration(seconds) * time.Second), Processes: ps}
	}
	complete := []sample{sampleAt(1, "web", "server", "postgres"), sampleAt(5, "web", "server", "postgres"), sampleAt(9, "web", "server", "postgres")}
	if !coverage(records, complete, []string{"web", "server", "postgres"}) {
		t.Fatal("complete coverage rejected")
	}
	missing := []sample{complete[0], sampleAt(5, "web", "server"), sampleAt(9, "web", "server")}
	if coverage(records, missing, []string{"web", "server", "postgres"}) {
		t.Fatal("missing process accepted")
	}
	if coverage(records, complete[:1], []string{"web", "server", "postgres"}) {
		t.Fatal("tail gap accepted")
	}
}

func TestReport_SeparateSamplerAndGeneratorCPU(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cpu := 10.0
	records := []loadrun.Record{{Phase: "A", Stream: "search", Status: 200, Start: at, DurationMS: 1000}, {Phase: "A", Stream: "reader", Status: 200, Start: at, DurationMS: 1000}}
	processes := map[string]processSample{}
	for _, name := range []string{"web", "server", "postgres", "loadcheck", "generator"} {
		processes[name] = processSample{Processes: []process{{PID: 1}}}
	}
	processes["loadcheck"] = processSample{CPU: 11, Processes: []process{{PID: 1}}}
	processes["generator"] = processSample{CPU: 33, Processes: []process{{PID: 2}}}
	samples := []sample{{Time: at.Add(time.Millisecond), HostCPU: &cpu, Processes: processes}}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, samples, strings.NewReader(""), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, g := range gates {
		found[g.Name] = g.Value
	}
	if found["sampler CPU peak"] != "11.00%" || found["generator CPU peak"] != "33.00%" {
		t.Fatalf("CPU rows %v", found)
	}
}

func TestReport_MissingGeneratorCPUIsUnavailable(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []loadrun.Record{{Phase: "A", Stream: "search", Status: 200, Start: at, DurationMS: 1000}}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, nil, strings.NewReader(""), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sampler CPU peak", "generator CPU peak"} {
		found := false
		for _, g := range gates {
			if g.Name == name {
				found = true
				if g.Result != "unavailable" {
					t.Errorf("%s: %s", name, g.Result)
				}
			}
		}
		if !found {
			t.Errorf("missing %s", name)
		}
	}
}

func TestReport_MissingCPUIntervalsUnavailable(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cpu := 10.0
	records := []loadrun.Record{{Phase: "A", Stream: "search", Status: 200, Start: start, DurationMS: 12000}}
	processes := map[string]processSample{}
	for _, name := range []string{"web", "server", "postgres"} {
		processes[name] = processSample{Processes: []process{{PID: 1}}}
	}
	samples := []sample{{Time: start.Add(time.Second), HostCPU: &cpu, Processes: processes}, {Time: start.Add(5 * time.Second), Processes: processes}, {Time: start.Add(9 * time.Second), Processes: processes}}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, samples, strings.NewReader(""), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == "Host CPU" {
			if g.Result != "unavailable" {
				t.Fatalf("CPU gate %s", g.Result)
			}
			return
		}
	}
	t.Fatal("missing CPU gate")
}
