package loadrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type recorder struct {
	mu          sync.Mutex
	file        *os.File
	records     []Record
	err         error
	activations []time.Time
}

func (r *recorder) add(v Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, v)
	if r.err == nil {
		r.err = json.NewEncoder(r.file).Encode(v)
	}
}
func (r *recorder) activation(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activations = append(r.activations, t)
}
func (r *recorder) report() Report {
	result := Report{Phases: map[string]map[string]Stream{}, Gates: map[string]string{}, Activations: r.activations}
	durations := map[string]map[string][]float64{}
	success := map[string]map[string][]float64{}
	errorsByStream := map[string]map[string]int{}
	for _, rec := range r.records {
		if result.Phases[rec.Phase] == nil {
			result.Phases[rec.Phase] = map[string]Stream{}
			durations[rec.Phase] = map[string][]float64{}
			success[rec.Phase] = map[string][]float64{}
			errorsByStream[rec.Phase] = map[string]int{}
		}
		stream := result.Phases[rec.Phase][rec.Stream]
		if stream.Status == nil {
			stream.Status = map[int]int{}
		}
		stream.Count++
		stream.Status[rec.Status]++
		stream.Retries += rec.Retries
		if rec.Aborted {
			stream.Aborts++
		}
		durations[rec.Phase][rec.Stream] = append(durations[rec.Phase][rec.Stream], rec.DurationMS)
		if rec.Status >= 200 && rec.Status < 300 && !rec.Aborted {
			success[rec.Phase][rec.Stream] = append(success[rec.Phase][rec.Stream], rec.DurationMS)
		} else if (rec.Stream == "search" || rec.Stream == "reader") || rec.Phase != "D" || (!rec.Aborted && rec.Status != 429 && rec.Status != 503) {
			errorsByStream[rec.Phase][rec.Stream]++
		}
		result.Phases[rec.Phase][rec.Stream] = stream
	}
	for phase, streams := range result.Phases {
		for name, s := range streams {
			values := durations[phase][name]
			s.P50MS = percentile(values, 50)
			s.P95MS = percentile(values, 95)
			s.P99MS = percentile(values, 99)
			streams[name] = s
		}
	}
	for phase, streams := range result.Phases {
		for _, name := range []string{"search", "reader"} {
			_, ok := streams[name]
			if !ok {
				continue
			}
			limit95, limit99 := 1000.0, 2000.0
			if name == "reader" {
				limit95, limit99 = 1500, 3000
			}
			p95 := percentile(success[phase][name], 95)
			p99 := percentile(success[phase][name], 99)
			pass := len(success[phase][name]) > 0 && p95 <= limit95 && p99 <= limit99
			if baseline := percentile(success["A"][name], 95); len(success["A"][name]) > 0 {
				pass = pass && p95 <= 1.25*baseline
			} else {
				result.Gates[phase+"/"+name+"/relative"] = "unavailable"
			}
			result.Gates[phase+"/"+name+"/latency"] = verdict(pass)
			result.Gates[phase+"/"+name+"/errors"] = verdict(errorsByStream[phase][name] == 0)
		}
	}
	if b, ok := result.Phases["B"]["summary"]; ok {
		result.Gates["B/coverage"] = verdict(b.Count > 0 && b.Status[200] == b.Count)
		result.Gates["B/summary/latency"] = verdict(b.Status[200] > 0 && percentile(success["B"]["summary"], 95) <= 500)
	}
	return result
}
func verdict(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}
func writeReport(out string, v Report) error {
	f, err := os.OpenFile(filepath.Join(out, "report.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(v); err != nil {
		_ = f.Close()
		return fmt.Errorf("write report: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close report: %w", err)
	}
	return nil
}
