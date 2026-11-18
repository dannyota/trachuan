package observe

import (
	"encoding/json"
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
	gates, err := buildReport(requests, loadrun.Report{}, nil, strings.NewReader(""), strings.NewReader(server), extras{})
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
	gates, err := buildReport(records, loadrun.Report{}, nil, strings.NewReader(web.String()), strings.NewReader(server), extras{})
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
		samples = append(samples, sample{Time: start.Add(time.Duration(seconds) * time.Second), HostCPU: &cpu, Processes: processes, Services: map[string]serviceSample{"web": {}, "server": {}}})
	}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:30Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, samples, strings.NewReader(""), strings.NewReader(server), extras{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"PostgreSQL connections", "D Host CPU", "web memory peak", "server memory peak", "Restarts"} {
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
	processes["loadcheck"] = processSample{LifetimeCPU: 11, Processes: []process{{PID: 1}}}
	processes["generator"] = processSample{LifetimeCPU: 33, Processes: []process{{PID: 2}}}
	samples := []sample{{Time: at.Add(time.Millisecond), HostCPU: &cpu, Processes: processes}}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, samples, strings.NewReader(""), strings.NewReader(server), extras{})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, g := range gates {
		found[g.Name] = g.Value
	}
	if found["sampler CPU lifetime average"] != "11.00%" || found["generator CPU lifetime average"] != "33.00%" {
		t.Fatalf("CPU rows %v", found)
	}
}

func TestReport_MissingGeneratorCPUIsUnavailable(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []loadrun.Record{{Phase: "A", Stream: "search", Status: 200, Start: at, DurationMS: 1000}}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, nil, strings.NewReader(""), strings.NewReader(server), extras{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sampler CPU lifetime average", "generator CPU lifetime average"} {
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
	gates, err := buildReport(records, loadrun.Report{}, samples, strings.NewReader(""), strings.NewReader(server), extras{})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == "A Host CPU" {
			if g.Result != "unavailable" {
				t.Fatalf("CPU gate %s", g.Result)
			}
			return
		}
	}
	t.Fatal("missing CPU gate")
}

func TestReport_ConnectionsUnavailableWithoutValues(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []loadrun.Record{{Phase: "A", Stream: "search", Status: 200, Start: at, DurationMS: 1000}, {Phase: "A", Stream: "reader", Status: 200, Start: at, DurationMS: 1000}}
	var s sample
	data := `{"time":"2026-01-01T00:00:00.1Z","host_cpu_percent":10,"processes":{"web":{"processes":[{"pid":1}]},"server":{"processes":[{"pid":2}]},"postgres":{"processes":[{"pid":3}]},"loadcheck":{"processes":[{"pid":4}]},"generator":{"processes":[{"pid":5}]}},"services":{"web":{"cpu_percent":5,"memory_peak_bytes":1},"server":{"cpu_percent":5,"memory_peak_bytes":1}}}`
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		t.Fatal(err)
	}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, []sample{s}, strings.NewReader(""), strings.NewReader(server), extras{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range gates {
		if g.Name == "PostgreSQL connections" {
			found = true
			if g.Result != "unavailable" || g.Value != "unavailable" {
				t.Errorf("connections gate = %+v", g)
			}
		} else if g.Result == "unavailable" {
			t.Errorf("unexpected unavailable gate: %+v", g)
		}
	}
	if !found {
		t.Fatal("missing connections gate")
	}
}
