package observe

import (
	"fmt"
	"math"
	"sort"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

const mib = 1 << 20

func unavailable(name, limit string) gate {
	return gate{Name: name, Value: "unavailable", Limit: limit, Result: "unavailable"}
}
func resourceGates(records []loadrun.Record, samples []sample, fileConnections []sample) []gate {
	ws := windows(records)
	var inRun []sample
	for _, s := range samples {
		if inAnyWindow(ws, s.Time) {
			inRun = append(inRun, s)
		}
	}
	var gates []gate
	gates = append(gates, hostCPUGates(ws, samples)...)
	gates = append(gates, serviceCPUGates(ws, inRun)...)
	gates = append(gates, memoryGates(ws, samples, inRun)...)
	gates = append(gates, connectionsGate(ws, append(append([]sample(nil), samples...), fileConnections...)))
	return append(gates, restartGates(records, samples, inRun)...)
}
func hostCPUGates(ws []window, samples []sample) []gate {
	var gates []gate
	for _, w := range ws {
		name, limit := w.phase+" Host CPU", "≤ 80%"
		if w.phase == "D" {
			limit = "recorded"
		}
		var in []sample
		for _, s := range samples {
			if s.HostCPU != nil && w.contains(s.Time) {
				in = append(in, s)
			}
		}
		if len(in) == 0 || !covered(w, samples, hasHostCPU) {
			gates = append(gates, unavailable(name, limit))
			continue
		}
		sort.Slice(in, func(i, j int) bool { return in[i].Time.Before(in[j].Time) })
		var peak, sum float64
		for _, s := range in {
			peak, sum = math.Max(peak, *s.HostCPU), sum+*s.HostCPU
		}
		mean := sum / float64(len(in))
		gated, value := mean, fmt.Sprintf("%.2f%% (mean, fewer than 60 samples)", mean)
		if len(in) >= 60 {
			gated = 0
			window := 0.0
			for i, s := range in {
				window += *s.HostCPU
				if i >= 60 {
					window -= *in[i-60].HostCPU
				}
				if i >= 59 {
					gated = math.Max(gated, window/60)
				}
			}
			value = fmt.Sprintf("%.2f%% (60 s rolling mean)", gated)
		}
		if w.phase == "D" {
			gates = append(gates, gate{Name: name, Value: value, Limit: limit, Result: "info"})
		} else {
			addGate(&gates, name, value, limit, gated <= 80)
		}
		gates = append(gates,
			gate{Name: name + " phase mean", Value: fmt.Sprintf("%.2f%%", mean), Limit: "recorded", Result: "info"},
			gate{Name: name + " 1 s peak", Value: fmt.Sprintf("%.2f%%", peak), Limit: "recorded", Result: "info"})
	}
	return gates
}
func runMemoryPeak(inRun []sample, name string) (uint64, bool) {
	var first, last serviceSample
	var firstAt, lastAt time.Time
	var peak uint64
	found := false
	for _, s := range inRun {
		v, ok := s.Services[name]
		if !ok {
			continue
		}
		peak = max(peak, v.Current)
		if !found || s.Time.Before(firstAt) {
			first, firstAt = v, s.Time
		}
		if !found || s.Time.After(lastAt) {
			last, lastAt = v, s.Time
		}
		found = true
	}
	if found && last.Peak > first.Peak {
		peak = max(peak, last.Peak)
	}
	return peak, found
}
func memoryGates(ws []window, samples, inRun []sample) []gate {
	limits := map[string]uint64{"web": 300 * mib, "server": 2156000000}
	names := map[string]bool{"web": true, "server": true}
	for _, s := range inRun {
		for name := range s.Services {
			names[name] = true
		}
	}
	var sorted []string
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	var gates []gate
	for _, name := range sorted {
		label, limit := name+" memory peak", uint64(0)
		gated := limits[name] > 0
		text := "recorded"
		if gated {
			limit = limits[name]
			text = fmt.Sprintf("≤ %.2f MiB", float64(limit)/mib)
		}
		peak, found := runMemoryPeak(inRun, name)
		if len(ws) == 0 || !found || !allCovered(ws, samples, hasService(name)) {
			gates = append(gates, unavailable(label, text))
			continue
		}
		value := fmt.Sprintf("%.2f MiB", float64(peak)/mib)
		if gated {
			addGate(&gates, label, value, text, peak <= limit)
		} else {
			gates = append(gates, gate{Name: label, Value: value, Limit: text, Result: "info"})
		}
	}
	return gates
}
func serviceCPUGates(ws []window, inRun []sample) []gate {
	names := map[string]bool{}
	for _, s := range inRun {
		for name := range s.Services {
			names[name] = true
		}
	}
	var sorted []string
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	var gates []gate
	for _, w := range ws {
		for _, name := range sorted {
			var peak, sum float64
			n := 0
			for _, s := range inRun {
				if v := s.Services[name]; v.CPU != nil && w.contains(s.Time) {
					peak, sum, n = math.Max(peak, *v.CPU), sum+*v.CPU, n+1
				}
			}
			label := w.phase + " " + name + " CPU"
			if n == 0 {
				gates = append(gates, unavailable(label, "recorded"))
				continue
			}
			gates = append(gates, gate{Name: label, Value: fmt.Sprintf("mean %.2f%%, peak %.2f%%", sum/float64(n), peak), Limit: "recorded", Result: "info"})
		}
	}
	return gates
}
func connectionsGate(ws []window, points []sample) gate {
	peak, n := 0, 0
	for _, s := range points {
		if s.Connections != nil && inAnyWindow(ws, s.Time) {
			peak, n = max(peak, *s.Connections), n+1
		}
	}
	if n == 0 || !allCovered(ws, points, hasConnections) {
		return unavailable("PostgreSQL connections", "≤ 40")
	}
	var gates []gate
	addGate(&gates, "PostgreSQL connections", fmt.Sprint(peak), "≤ 40", peak <= 40)
	return gates[0]
}
func restartGates(records []loadrun.Record, samples, inRun []sample) []gate {
	names := map[string]bool{"web": true, "server": true, "postgres": true}
	for _, s := range samples {
		for name := range s.Processes {
			names[name] = true
		}
		for name := range s.Services {
			names[name] = true
		}
	}
	var required []string
	for name := range names {
		required = append(required, name)
	}
	complete := coverage(records, samples, required)
	restarts := 0
	maxLifetime := map[string]float64{}
	observed := map[string]bool{}
	for _, s := range inRun {
		for name, p := range s.Processes {
			if len(p.Processes) > 0 {
				observed[name] = true
			}
			maxLifetime[name] = math.Max(maxLifetime[name], p.LifetimeCPU)
			if p.Restart {
				restarts++
			}
		}
		for _, v := range s.Services {
			if v.Restart {
				restarts++
			}
		}
	}
	var gates []gate
	for _, name := range []string{"loadcheck", "generator"} {
		label := "generator"
		if name == "loadcheck" {
			label = "sampler"
		}
		label += " CPU lifetime average"
		if observed[name] {
			gates = append(gates, gate{Name: label, Value: fmt.Sprintf("%.2f%%", maxLifetime[name]), Limit: "recorded", Result: "info"})
		} else {
			gates = append(gates, unavailable(label, "recorded"))
		}
	}
	addGate(&gates, "Restarts", fmt.Sprint(restarts), "0", len(inRun) > 0 && restarts == 0)
	if !complete {
		for i := range gates {
			if gates[i].Name != "Restarts" || restarts == 0 {
				gates[i].Result = "unavailable"
			}
		}
	}
	return gates
}
