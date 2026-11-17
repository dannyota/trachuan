package observe

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

type gate struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Limit  string `json:"limit"`
	Result string `json:"result"`
}

func CommandReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var run, samples, web, server, out string
	fs.StringVar(&run, "run", "", "run directory")
	fs.StringVar(&samples, "samples", "", "samples JSONL")
	fs.StringVar(&web, "web-log", "", "web log")
	fs.StringVar(&server, "server-log", "", "server log")
	fs.StringVar(&out, "out", "", "output JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || run == "" || samples == "" || web == "" || server == "" || out == "" {
		return errors.New("invalid report options")
	}
	records, err := readRecords(filepath.Join(run, "requests.jsonl"))
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return errors.New("run has no requests")
	}
	var summary loadrun.Report
	f, err := os.Open(filepath.Join(run, "report.json"))
	if err != nil {
		return errors.New("open run report failed")
	}
	err = json.NewDecoder(f).Decode(&summary)
	_ = f.Close()
	if err != nil {
		return errors.New("read run report failed")
	}
	measurements, err := readSamples(samples)
	if err != nil {
		return err
	}
	webFile, err := os.Open(web)
	if err != nil {
		return errors.New("open web log failed")
	}
	defer func() { _ = webFile.Close() }()
	serverFile, err := os.Open(server)
	if err != nil {
		return errors.New("open server log failed")
	}
	defer func() { _ = serverFile.Close() }()
	gates, err := buildReport(records, summary, measurements, webFile, serverFile)
	if err != nil {
		return err
	}
	dest, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	encoder := json.NewEncoder(dest)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(struct {
		Gates []gate `json:"gates"`
	}{gates}); err != nil {
		_ = dest.Close()
		return errors.New("write report failed")
	}
	if err = dest.Close(); err != nil {
		return errors.New("close report failed")
	}
	fmt.Println("| Gate | Observed | Limit | Result |\n| --- | --- | --- | --- |")
	for _, g := range gates {
		fmt.Printf("| %s | %s | %s | %s |\n", g.Name, g.Value, g.Limit, g.Result)
	}
	return nil
}
func readRecords(path string) ([]loadrun.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open requests failed")
	}
	defer func() { _ = f.Close() }()
	var result []loadrun.Record
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 4<<20)
	for scan.Scan() {
		var r loadrun.Record
		if json.Unmarshal(scan.Bytes(), &r) != nil || r.Start.IsZero() || r.DurationMS < 0 {
			return nil, errors.New("invalid request record")
		}
		result = append(result, r)
	}
	if scan.Err() != nil {
		return nil, errors.New("read requests failed")
	}
	return result, nil
}
func readSamples(path string) ([]sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open samples failed")
	}
	defer func() { _ = f.Close() }()
	var result []sample
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 4<<20)
	for scan.Scan() {
		var s sample
		if json.Unmarshal(scan.Bytes(), &s) != nil || s.Time.IsZero() {
			return nil, errors.New("invalid sample")
		}
		result = append(result, s)
	}
	if scan.Err() != nil {
		return nil, errors.New("read samples failed")
	}
	return result, nil
}
func quantile(values []float64, p int) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	return values[int(math.Ceil(float64(len(values))*float64(p)/100))-1]
}
func addGate(gates *[]gate, name, value, limit string, ok bool) {
	result := "fail"
	if ok {
		result = "pass"
	}
	*gates = append(*gates, gate{Name: name, Value: value, Limit: limit, Result: result})
}
func ms(v float64) string { return fmt.Sprintf("%.2f ms", v) }
func bounds(records []loadrun.Record, phase string) (time.Time, time.Time) {
	var start, end time.Time
	for _, r := range records {
		if phase != "" && phase != r.Phase {
			continue
		}
		finish := r.Start.Add(time.Duration(r.DurationMS * float64(time.Millisecond)))
		if start.IsZero() || r.Start.Before(start) {
			start = r.Start
		}
		if end.IsZero() || finish.After(end) {
			end = finish
		}
	}
	return start, end
}
func buildReport(records []loadrun.Record, run loadrun.Report, samples []sample, web, server io.Reader) ([]gate, error) {
	start, end := bounds(records, "")
	cStart, cEnd := bounds(records, "C")
	counts, err := renders(web, cStart, cEnd)
	if err != nil {
		return nil, err
	}
	var gates []gate
	durations := map[string]map[string][]float64{}
	signedErrors := map[string]int{}
	var cold []float64
	second := map[string]loadrun.Record{}
	for _, r := range records {
		if durations[r.Phase] == nil {
			durations[r.Phase] = map[string][]float64{}
		}
		if r.Stream == "search" || r.Stream == "reader" {
			if r.Status >= 200 && r.Status < 300 && !r.Aborted {
				durations[r.Phase][r.Stream] = append(durations[r.Phase][r.Stream], r.DurationMS)
			} else {
				signedErrors[r.Phase]++
			}
		}
		if r.Phase == "B" && r.Stream == "summary" && r.Status == 200 {
			cold = append(cold, r.DurationMS)
		}
		if r.Phase == "C" && r.Stream == "summary" && r.Pass == 2 {
			second[r.Key] = r
		}
	}
	for _, phase := range []string{"A", "B", "C", "D"} {
		streams, exists := durations[phase]
		if !exists {
			continue
		}
		for _, name := range []string{"search", "reader"} {
			values := streams[name]
			baseline := durations["A"][name]
			limit95, limit99 := 1000.0, 2000.0
			if name == "reader" {
				limit95, limit99 = 1500, 3000
			}
			p95, p99 := quantile(values, 95), quantile(values, 99)
			base := quantile(baseline, 95)
			addGate(&gates, phase+" "+name, ms(p95)+" p95, "+ms(p99)+" p99", fmt.Sprintf("p95 ≤ %.0f ms, p99 ≤ %.0f ms, p95 ≤ 1.25 × A", limit95, limit99), len(values) > 0 && len(baseline) > 0 && latencyGate(p95, p99, base, limit95, limit99))
			if len(baseline) == 0 {
				gates[len(gates)-1].Result = "unavailable"
			}
		}
		addGate(&gates, phase+" signed-in errors", fmt.Sprint(signedErrors[phase]), "0", signedErrors[phase] == 0)
	}
	if b, ok := run.Phases["B"]["summary"]; ok {
		addGate(&gates, "B coverage", fmt.Sprintf("%d/%d", b.Status[200], b.Count), "all", b.Count > 0 && b.Status[200] == b.Count)
		addGate(&gates, "B summary p95", ms(quantile(cold, 95)), "≤ 500 ms", len(cold) > 0 && quantile(cold, 95) <= 500)
	}
	if !cStart.IsZero() {
		hits := 0
		var hitLatency []float64
		for key, r := range second {
			if r.Status == 200 && counts[key] <= 1 {
				hits++
				hitLatency = append(hitLatency, r.DurationMS)
			}
		}
		rate := 0.0
		if len(second) > 0 {
			rate = 100 * float64(hits) / float64(len(second))
		}
		addGate(&gates, "C hit rate", fmt.Sprintf("%.2f%% (%d/%d)", rate, hits, len(second)), "≥ 99%", len(second) == 500 && rate >= 99)
		addGate(&gates, "C hit p95", ms(quantile(hitLatency, 95)), "≤ 50 ms", len(hitLatency) > 0 && quantile(hitLatency, 95) <= 50)
	}
	wait, err := poolP99(server, start, end)
	if err != nil {
		return nil, err
	}
	addGate(&gates, "Reader pool p99", ms(wait), "≤ 50 ms", wait <= 50)
	names := map[string]bool{"web": true, "server": true, "postgres": true}
	for _, s := range samples {
		for name := range s.Processes {
			names[name] = true
		}
	}
	var required []string
	for name := range names {
		required = append(required, name)
	}
	complete := coverage(records, samples, required)
	var cpuSamples []sample
	for _, s := range samples {
		if s.HostCPU != nil {
			cpuSamples = append(cpuSamples, s)
		}
	}
	cpuComplete := coverage(records, cpuSamples, nil)
	peakStart := len(gates)
	var connections int
	var cpu float64
	haveCPU := false
	maxRSS := map[string]uint64{}
	maxCPU := map[string]float64{}
	restarts := 0
	sampleCount := 0
	observed := map[string]bool{}
	for _, s := range samples {
		if s.Time.Before(start) || s.Time.After(end) {
			continue
		}
		sampleCount++
		connections = max(connections, s.Connections)
		if s.HostCPU != nil {
			cpu = math.Max(cpu, *s.HostCPU)
			haveCPU = true
		}
		for name, p := range s.Processes {
			if len(p.Processes) > 0 {
				observed[name] = true
			}
			maxRSS[name] = max(maxRSS[name], p.RSS)
			maxCPU[name] = math.Max(maxCPU[name], p.CPU)
			if p.Restart {
				restarts++
			}
		}
	}
	addGate(&gates, "PostgreSQL connections", fmt.Sprint(connections), "≤ 40", sampleCount > 0 && connections <= 40)
	addGate(&gates, "Host CPU", fmt.Sprintf("%.2f%%", cpu), "≤ 80%", haveCPU && cpu <= 80)
	for _, entry := range []struct {
		name  string
		limit uint64
	}{{"web", 300000000}, {"server", 2156000000}} {
		addGate(&gates, entry.name+" RSS", fmt.Sprintf("%.2f MB", float64(maxRSS[entry.name])/1e6), fmt.Sprintf("≤ %d MB", entry.limit/1000000), observed[entry.name] && maxRSS[entry.name] <= entry.limit)
	}
	for _, name := range []string{"web", "server", "postgres", "loadcheck", "generator"} {
		label := name
		if name == "loadcheck" {
			label = "sampler"
		}
		if observed[name] {
			gates = append(gates, gate{Name: label + " CPU peak", Value: fmt.Sprintf("%.2f%%", maxCPU[name]), Limit: "recorded", Result: "info"})
		} else if name == "loadcheck" || name == "generator" {
			gates = append(gates, gate{Name: label + " CPU peak", Value: "unavailable", Limit: "recorded", Result: "unavailable"})
		}
	}
	if observed["postgres"] {
		gates = append(gates, gate{Name: "postgres RSS peak", Value: fmt.Sprintf("%.2f MB", float64(maxRSS["postgres"])/1e6), Limit: "recorded", Result: "info"})
	}
	addGate(&gates, "Restarts", fmt.Sprint(restarts), "0", sampleCount > 0 && observed["web"] && observed["server"] && observed["postgres"] && restarts == 0)
	if !complete {
		for i := peakStart; i < len(gates); i++ {
			gates[i].Result = "unavailable"
		}
	}
	if !cpuComplete {
		for i := peakStart; i < len(gates); i++ {
			if gates[i].Name == "Host CPU" {
				gates[i].Result = "unavailable"
			}
		}
	}
	return gates, nil
}
