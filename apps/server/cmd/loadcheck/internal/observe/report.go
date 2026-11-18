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
	var run, samples, web, server, out, baseline, connections string
	fs.StringVar(&run, "run", "", "run directory")
	fs.StringVar(&samples, "samples", "", "samples JSONL")
	fs.StringVar(&web, "web-log", "", "web log")
	fs.StringVar(&server, "server-log", "", "server log")
	fs.StringVar(&out, "out", "", "output JSON")
	fs.StringVar(&baseline, "baseline", "", "run directory that supplies phase A when the run has none")
	fs.StringVar(&connections, "connections", "", "connections.txt, one '<RFC3339 UTC> <count>' line per second")
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
	var extra extras
	if baseline != "" {
		if extra.baseline, err = readRecords(filepath.Join(baseline, "requests.jsonl")); err != nil {
			return err
		}
	}
	if connections != "" {
		if extra.connections, err = readConnections(connections); err != nil {
			return err
		}
	}
	gates, err := buildReport(records, summary, measurements, webFile, serverFile, extra)
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
