package observe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const poolLog = `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`

func phaseRecords(phase string, at time.Time, searchMS, readerMS float64) []loadrun.Record {
	return []loadrun.Record{
		{Phase: phase, Stream: "search", Status: 200, Start: at, DurationMS: searchMS},
		{Phase: phase, Stream: "reader", Status: 200, Start: at, DurationMS: readerMS},
	}
}

func spread(phase string, at time.Time, fast, slow float64, nslow int) []loadrun.Record {
	var out []loadrun.Record
	for i := 0; i < 100; i++ {
		d := fast
		if i >= 100-nslow {
			d = slow
		}
		out = append(out, loadrun.Record{Phase: phase, Stream: "search", Status: 200, Start: at, DurationMS: d})
	}
	return append(out, loadrun.Record{Phase: phase, Stream: "reader", Status: 200, Start: at, DurationMS: 100})
}

func report(t *testing.T, records []loadrun.Record, samples []sample, extra extras) map[string]gate {
	t.Helper()
	gates, err := buildReport(records, loadrun.Report{}, samples, strings.NewReader(""), strings.NewReader(poolLog), extra)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]gate{}
	for _, g := range gates {
		out[g.Name] = g
	}
	return out
}

func want(t *testing.T, gates map[string]gate, name, result string) gate {
	t.Helper()
	g, ok := gates[name]
	if !ok {
		t.Fatalf("missing gate %q", name)
	}
	if g.Result != result {
		t.Fatalf("%s = %+v, want %s", name, g, result)
	}
	return g
}

func TestReport_SearchAbsoluteBoundsByPhase(t *testing.T) {
	records := append(phaseRecords("A", t0, 1000, 100), phaseRecords("B", t0.Add(time.Minute), 1250, 100)...)
	gates := report(t, records, nil, extras{})
	want(t, gates, "A search", "pass")
	want(t, gates, "B search", "pass")
	gates = report(t, phaseRecords("A", t0, 1001, 100), nil, extras{})
	want(t, gates, "A search", "fail")
	records = append(phaseRecords("A", t0, 1000, 100), phaseRecords("B", t0.Add(time.Minute), 1251, 100)...)
	want(t, report(t, records, nil, extras{}), "B search", "fail")
}

func TestReport_SearchP99Bounds(t *testing.T) {
	a := spread("A", t0, 100, 2001, 2)
	want(t, report(t, a, nil, extras{}), "A search", "fail")
	a = spread("A", t0, 100, 2000, 2)
	want(t, report(t, a, nil, extras{}), "A search", "pass")
	b := append(spread("A", t0, 100, 100, 0), spread("B", t0.Add(time.Minute), 100, 2500, 2)...)
	want(t, report(t, b, nil, extras{}), "B search", "pass")
	b = append(spread("A", t0, 100, 100, 0), spread("B", t0.Add(time.Minute), 100, 2501, 2)...)
	want(t, report(t, b, nil, extras{}), "B search", "fail")
}

func TestReport_RelativeBoundStaysInEveryPhase(t *testing.T) {
	for _, phase := range []string{"B", "C", "D"} {
		records := append(phaseRecords("A", t0, 100, 100), phaseRecords(phase, t0.Add(time.Minute), 125, 100)...)
		want(t, report(t, records, nil, extras{}), phase+" search", "pass")
		records = append(phaseRecords("A", t0, 100, 100), phaseRecords(phase, t0.Add(time.Minute), 126, 100)...)
		want(t, report(t, records, nil, extras{}), phase+" search", "fail")
	}
}

func TestReport_DOnlyUsesBaselineRun(t *testing.T) {
	records := phaseRecords("D", t0, 120, 120)
	base := extras{baseline: phaseRecords("A", t0.Add(-time.Hour), 100, 100)}
	gates := report(t, records, nil, base)
	want(t, gates, "D search", "pass")
	want(t, gates, "D reader", "pass")
	base = extras{baseline: phaseRecords("A", t0.Add(-time.Hour), 90, 100)}
	want(t, report(t, records, nil, base), "D search", "fail")
	want(t, report(t, records, nil, extras{}), "D search", "unavailable")
	want(t, report(t, records, nil, extras{baseline: phaseRecords("B", t0, 100, 100)}), "D search", "unavailable")
}

func TestReport_RunsOwnAWinsOverBaseline(t *testing.T) {
	records := append(phaseRecords("A", t0, 100, 100), phaseRecords("D", t0.Add(time.Minute), 120, 100)...)
	base := extras{baseline: phaseRecords("A", t0.Add(-time.Hour), 10, 10)}
	want(t, report(t, records, nil, base), "D search", "pass")
}

func hostSample(at time.Time, cpu float64) sample {
	return sample{Time: at, HostCPU: &cpu, Processes: map[string]processSample{}}
}

func cpuSeries(values func(i int) float64, n int) []sample {
	var out []sample
	for i := 0; i < n; i++ {
		out = append(out, hostSample(t0.Add(time.Duration(i)*time.Second), values(i)))
	}
	return out
}

func TestReport_HostCPURollingMean(t *testing.T) {
	records := phaseRecords("A", t0, 69000, 100)
	spike := cpuSeries(func(i int) float64 {
		if i == 30 {
			return 100
		}
		return 10
	}, 70)
	gates := report(t, records, spike, extras{})
	g := want(t, gates, "A Host CPU", "pass")
	if !strings.Contains(g.Value, "60 s rolling mean") {
		t.Fatalf("%+v", g)
	}
	if g := want(t, gates, "A Host CPU 1 s peak", "info"); g.Value != "100.00%" {
		t.Fatalf("%+v", g)
	}
	want(t, gates, "A Host CPU phase mean", "info")
	sustained := cpuSeries(func(i int) float64 {
		if i >= 5 && i < 65 {
			return 90
		}
		return 0
	}, 70)
	want(t, report(t, records, sustained, extras{}), "A Host CPU", "fail")
	edge := cpuSeries(func(i int) float64 {
		if i >= 5 && i < 65 {
			return 80
		}
		return 0
	}, 70)
	want(t, report(t, records, edge, extras{}), "A Host CPU", "pass")
}

func TestReport_HostCPUShortPhaseUsesMean(t *testing.T) {
	records := phaseRecords("A", t0, 1000, 100)
	samples := []sample{hostSample(t0.Add(50*time.Millisecond), 90), hostSample(t0.Add(500*time.Millisecond), 80)}
	g := want(t, report(t, records, samples, extras{}), "A Host CPU", "fail")
	if !strings.Contains(g.Value, "85.00%") || !strings.Contains(g.Value, "fewer than 60 samples") {
		t.Fatalf("%+v", g)
	}
}

func TestReport_HostCPUInDIsInfo(t *testing.T) {
	records := phaseRecords("D", t0, 1000, 100)
	samples := []sample{hostSample(t0.Add(50*time.Millisecond), 95), hostSample(t0.Add(500*time.Millisecond), 99)}
	want(t, report(t, records, samples, extras{baseline: phaseRecords("A", t0, 1000, 100)}), "D Host CPU", "info")
}

func TestReport_ActivationMinutesExcludedFromRelative(t *testing.T) {
	acts := loadrun.Report{Activations: []time.Time{t0.Add(10 * time.Second), t0.Add(130 * time.Second)}}
	search := func(at int, ms float64) loadrun.Record {
		return loadrun.Record{Phase: "D", Stream: "search", Status: 200, Start: t0.Add(time.Duration(at) * time.Second), DurationMS: ms}
	}
	base := phaseRecords("A", t0.Add(-time.Hour), 100, 100)
	run := func(r loadrun.Report, records ...loadrun.Record) map[string]gate {
		gates, err := buildReport(records, r, nil, strings.NewReader(""), strings.NewReader(strings.Replace(poolLog, ":00:00Z", ":00:20Z", 1)), extras{baseline: base})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]gate{}
		for _, g := range gates {
			out[g.Name] = g
		}
		return out
	}
	slowInside := []loadrun.Record{search(5, 100), search(15, 1200), search(135, 1200), search(80, 110), search(200, 110)}
	g := want(t, run(acts, slowInside...), "D search", "pass")
	if !strings.Contains(g.Limit, "activation minutes excluded") {
		t.Fatalf("%+v", g)
	}
	want(t, run(loadrun.Report{}, slowInside...), "D search", "fail")
	slowOutside := []loadrun.Record{search(5, 100), search(15, 100), search(80, 200)}
	want(t, run(acts, slowOutside...), "D search", "fail")
	absolute := []loadrun.Record{search(5, 100), search(15, 1300), search(80, 100)}
	want(t, run(acts, absolute...), "D search", "fail")
	allInside := []loadrun.Record{search(15, 100), search(69, 100)}
	want(t, run(acts, allInside...), "D search", "unavailable")
	boundary := []loadrun.Record{search(10, 1200), search(70, 100)}
	want(t, run(acts, boundary...), "D search", "pass")
}

func TestReport_SamplesOutsidePhasesIgnored(t *testing.T) {
	records := phaseRecords("A", t0, 100, 100)
	high := 99.0
	samples := []sample{
		{Time: t0.Add(-time.Minute), HostCPU: &high, Services: map[string]serviceSample{"web": {Peak: 900 << 20, Restart: true}}},
		hostSample(t0.Add(50*time.Millisecond), 10),
		{Time: t0.Add(time.Hour), HostCPU: &high, Services: map[string]serviceSample{"web": {Peak: 900 << 20, Restart: true}}},
	}
	gates := report(t, records, samples, extras{})
	want(t, gates, "A Host CPU", "pass")
	want(t, gates, "web memory peak", "unavailable")
	if g := gates["Restarts"]; g.Value != "0" {
		t.Fatalf("restarts %+v", g)
	}
}

func serviceSamples(at time.Time, services map[string]serviceSample) sample {
	return sample{Time: at, Services: services}
}

func memSample(at time.Time, current, peak uint64, names ...string) sample {
	services := map[string]serviceSample{}
	for _, name := range names {
		services[name] = serviceSample{Current: current, Peak: peak}
	}
	return serviceSamples(at, services)
}

func TestReport_WebAndServerMemoryFromCurrent(t *testing.T) {
	records := phaseRecords("A", t0, 1000, 100)
	mk := func(web, server uint64) []sample {
		a := t0.Add(100 * time.Millisecond)
		b := t0.Add(900 * time.Millisecond)
		return []sample{
			serviceSamples(a, map[string]serviceSample{"web": {Current: web, Peak: web}, "server": {Current: server, Peak: server}}),
			serviceSamples(b, map[string]serviceSample{"web": {Current: 1, Peak: web}, "server": {Current: 1, Peak: server}}),
		}
	}
	gates := report(t, records, mk(300000000, 2156000000), extras{})
	g := want(t, gates, "web memory peak", "pass")
	if g.Value != "300.00 MB (286.10 MiB)" || g.Limit != "≤ 300 MB" {
		t.Fatalf("web %+v", g)
	}
	if g := want(t, gates, "server memory peak", "pass"); g.Limit != "≤ 2156 MB" {
		t.Fatalf("server %+v", g)
	}
	gates = report(t, records, mk(300000001, 2156000001), extras{})
	want(t, gates, "web memory peak", "fail")
	want(t, gates, "server memory peak", "fail")
	want(t, report(t, records, nil, extras{}), "web memory peak", "unavailable")
}

func TestReport_MemoryPeakSetBeforeRunIsIgnored(t *testing.T) {
	records := phaseRecords("A", t0, 1000, 100)
	samples := []sample{
		memSample(t0.Add(100*time.Millisecond), 100<<20, 500<<20, "web", "queryembed"),
		memSample(t0.Add(900*time.Millisecond), 120<<20, 500<<20, "web", "queryembed"),
	}
	gates := report(t, records, samples, extras{})
	if g := want(t, gates, "web memory peak", "pass"); g.Value != "125.83 MB (120.00 MiB)" {
		t.Fatalf("%+v", g)
	}
	if g := want(t, gates, "queryembed memory peak", "info"); g.Value != "125.83 MB (120.00 MiB)" {
		t.Fatalf("%+v", g)
	}
}

func TestReport_MemoryPeakSetDuringRunCounts(t *testing.T) {
	records := phaseRecords("A", t0, 1000, 100)
	samples := []sample{
		memSample(t0.Add(100*time.Millisecond), 100<<20, 150<<20, "web", "postgres", "caddy"),
		memSample(t0.Add(900*time.Millisecond), 120<<20, 400<<20, "web", "postgres", "caddy"),
	}
	gates := report(t, records, samples, extras{})
	if g := want(t, gates, "web memory peak", "fail"); g.Value != "419.43 MB (400.00 MiB)" {
		t.Fatalf("%+v", g)
	}
	for _, name := range []string{"postgres", "caddy"} {
		if g := want(t, gates, name+" memory peak", "info"); g.Value != "419.43 MB (400.00 MiB)" {
			t.Fatalf("%+v", g)
		}
	}
}

func TestReport_ServiceCPUInfoPerPhase(t *testing.T) {
	cpu := func(v float64) *float64 { return &v }
	records := append(phaseRecords("A", t0, 1000, 100), phaseRecords("B", t0.Add(time.Minute), 1000, 100)...)
	samples := []sample{
		serviceSamples(t0.Add(100*time.Millisecond), map[string]serviceSample{"queryembed": {CPU: cpu(10)}}),
		serviceSamples(t0.Add(900*time.Millisecond), map[string]serviceSample{"queryembed": {CPU: cpu(30)}}),
		serviceSamples(t0.Add(time.Minute+100*time.Millisecond), map[string]serviceSample{"queryembed": {CPU: cpu(50)}}),
		serviceSamples(t0.Add(time.Minute+900*time.Millisecond), map[string]serviceSample{"queryembed": {}}),
	}
	gates := report(t, records, samples, extras{})
	if g := want(t, gates, "A queryembed CPU", "info"); g.Value != "mean 20.00%, peak 30.00%" {
		t.Fatalf("A %+v", g)
	}
	if g := want(t, gates, "B queryembed CPU", "info"); g.Value != "mean 50.00%, peak 50.00%" {
		t.Fatalf("B %+v", g)
	}
}

func TestReport_ConnectionsFilePeak(t *testing.T) {
	records := phaseRecords("A", t0, 1000, 100)
	points := func(counts ...int) extras {
		var e extras
		for i, c := range counts {
			c := c
			e.connections = append(e.connections, sample{Time: t0.Add(time.Duration(i) * 400 * time.Millisecond), Connections: &c})
		}
		return e
	}
	want(t, report(t, records, nil, points(3, 40, 5)), "PostgreSQL connections", "pass")
	g := want(t, report(t, records, nil, points(3, 41, 5)), "PostgreSQL connections", "fail")
	if g.Value != "41" {
		t.Fatalf("%+v", g)
	}
	outside := points(3, 5)
	high := 90
	outside.connections = append(outside.connections, sample{Time: t0.Add(time.Hour), Connections: &high})
	want(t, report(t, records, nil, outside), "PostgreSQL connections", "pass")
	want(t, report(t, records, nil, extras{}), "PostgreSQL connections", "unavailable")
}

func TestCommand_ReportWithBaselineAndConnections(t *testing.T) {
	dir := t.TempDir()
	write := func(path, content string) string {
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("run/requests.jsonl", `{"phase":"D","stream":"search","start":"2026-01-01T00:00:00Z","duration_ms":120,"status":200}`+"\n"+`{"phase":"D","stream":"reader","start":"2026-01-01T00:00:00Z","duration_ms":120,"status":200}`+"\n")
	write("run/report.json", "{}")
	write("base/requests.jsonl", `{"phase":"A","stream":"search","start":"2026-01-01T00:00:00Z","duration_ms":100,"status":200}`+"\n"+`{"phase":"A","stream":"reader","start":"2026-01-01T00:00:00Z","duration_ms":100,"status":200}`+"\n")
	samples := write("samples.jsonl", `{"time":"2026-01-01T00:00:00.05Z","host_cpu_percent":10}`+"\n")
	conns := write("connections.txt", "2026-01-01T00:00:00Z 7\n")
	webLog := write("web.log", "")
	serverLog := write("server.log", poolLog+"\n")
	out := filepath.Join(dir, "gates.json")
	args := []string{"--run", filepath.Join(dir, "run"), "--samples", samples, "--web-log", webLog, "--server-log", serverLog, "--out", out, "--baseline", filepath.Join(dir, "base"), "--connections", conns}
	if err := CommandReport(args); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Gates []gate }
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	results := map[string]gate{}
	for _, g := range got.Gates {
		results[g.Name] = g
	}
	want(t, results, "D search", "pass")
	if g := want(t, results, "PostgreSQL connections", "pass"); g.Value != "7" {
		t.Fatalf("%+v", g)
	}
	write("connections.txt", "garbage\n")
	args[9] = filepath.Join(dir, "gates2.json")
	if err := CommandReport(args); err == nil {
		t.Fatal("malformed connections accepted")
	}
}

func TestReport_VanishedServiceBlocksRestartsPass(t *testing.T) {
	records := phaseRecords("A", t0, 10000, 100)
	var samples []sample
	for i := 0; i <= 10; i += 2 {
		services := map[string]serviceSample{"web": {}, "server": {}, "postgres": {}}
		if i == 0 {
			services["caddy"] = serviceSample{}
		}
		samples = append(samples, serviceSamples(t0.Add(time.Duration(i)*time.Second), services))
	}
	if g := report(t, records, samples, extras{})["Restarts"]; g.Result == "pass" {
		t.Fatalf("%+v", g)
	}
}

func TestReport_AbsoluteFailureNotHiddenByMissingRelative(t *testing.T) {
	d := phaseRecords("D", t0, 5000, 100)
	want(t, report(t, d, nil, extras{}), "D search", "fail")
	want(t, report(t, phaseRecords("D", t0, 100, 100), nil, extras{}), "D search", "unavailable")
	acts := loadrun.Report{Activations: []time.Time{t0.Add(-time.Second)}}
	gates, err := buildReport(d, acts, nil, strings.NewReader(""), strings.NewReader(poolLog), extras{baseline: phaseRecords("A", t0, 100, 100)})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == "D search" && g.Result != "fail" {
			t.Fatalf("%+v", g)
		}
	}
}

func TestReport_RestartWithGapStillFails(t *testing.T) {
	records := phaseRecords("A", t0, 10000, 100)
	var samples []sample
	for _, i := range []int{0, 2, 10} {
		services := map[string]serviceSample{"web": {}, "server": {}, "postgres": {}, "caddy": {Restart: i == 10}}
		samples = append(samples, serviceSamples(t0.Add(time.Duration(i)*time.Second), services))
	}
	want(t, report(t, records, samples, extras{}), "Restarts", "fail")
}
