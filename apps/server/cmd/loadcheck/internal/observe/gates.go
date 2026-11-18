package observe

import (
	"fmt"
	"io"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

type extras struct {
	baseline    []loadrun.Record
	connections []sample
}

func successDurations(records []loadrun.Record) (map[string]map[string][]float64, map[string]int) {
	durations := map[string]map[string][]float64{}
	signedErrors := map[string]int{}
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
	}
	return durations, signedErrors
}
func outsideActivations(records []loadrun.Record, activations []time.Time) ([]loadrun.Record, int) {
	var kept []loadrun.Record
	excluded := 0
	for _, r := range records {
		inside := false
		for _, a := range activations {
			if !r.Start.Before(a) && r.Start.Before(a.Add(time.Minute)) {
				inside = true
			}
		}
		if inside {
			excluded++
		} else {
			kept = append(kept, r)
		}
	}
	return kept, excluded
}
func buildReport(records []loadrun.Record, run loadrun.Report, samples []sample, web, server io.Reader, extra extras) ([]gate, error) {
	start, end := bounds(records, "")
	cStart, cEnd := bounds(records, "C")
	counts, err := renders(web, cStart, cEnd)
	if err != nil {
		return nil, err
	}
	var gates []gate
	durations, signedErrors := successDurations(records)
	baselineA, _ := successDurations(extra.baseline)
	outside, _ := outsideActivations(records, run.Activations)
	relative, _ := successDurations(outside)
	var cold []float64
	second := map[string]loadrun.Record{}
	for _, r := range records {
		if r.Phase == "B" && r.Stream == "summary" && r.Status == 200 {
			cold = append(cold, r.DurationMS)
		}
		if r.Phase == "C" && r.Stream == "summary" && r.Pass == 2 {
			second[r.Key] = r
		}
	}
	for _, phase := range phaseNames {
		streams, exists := durations[phase]
		if !exists {
			continue
		}
		for _, name := range []string{"search", "reader"} {
			values := streams[name]
			baseline := durations["A"][name]
			if len(baseline) == 0 {
				baseline = baselineA["A"][name]
			}
			limit95, limit99 := loadrun.Limits(phase, name)
			p95, p99 := quantile(values, 95), quantile(values, 99)
			base := quantile(baseline, 95)
			relValues := relative[phase][name]
			note := ""
			if len(relValues) < len(values) {
				note = " (activation minutes excluded)"
			}
			pass := len(values) > 0 && p95 <= limit95 && p99 <= limit99 && len(relValues) > 0 && quantile(relValues, 95) <= base*1.25
			addGate(&gates, phase+" "+name, ms(p95)+" p95, "+ms(p99)+" p99", fmt.Sprintf("p95 ≤ %.0f ms, p99 ≤ %.0f ms, p95 ≤ 1.25 × A%s", limit95, limit99, note), pass && len(baseline) > 0)
			if len(baseline) == 0 || (len(values) > 0 && len(relValues) == 0) {
				gates[len(gates)-1].Result = "unavailable"
				if len(values) == 0 || p95 > limit95 || p99 > limit99 {
					gates[len(gates)-1].Result = "fail"
				}
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
	return append(gates, resourceGates(records, samples, extra.connections)...), nil
}
