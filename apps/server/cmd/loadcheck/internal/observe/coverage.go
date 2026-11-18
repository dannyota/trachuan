package observe

import (
	"sort"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

var phaseNames = []string{"A", "B", "C", "D"}

type window struct {
	phase      string
	start, end time.Time
}

func windows(records []loadrun.Record) []window {
	var result []window
	for _, phase := range phaseNames {
		if start, end := bounds(records, phase); !start.IsZero() {
			result = append(result, window{phase, start, end})
		}
	}
	return result
}
func (w window) contains(t time.Time) bool { return !t.Before(w.start) && !t.After(w.end) }
func inAnyWindow(ws []window, t time.Time) bool {
	for _, w := range ws {
		if w.contains(t) {
			return true
		}
	}
	return false
}
func hasProcess(name string) func(sample) bool {
	return func(s sample) bool {
		_, service := s.Services[name]
		return len(s.Processes[name].Processes) > 0 || service
	}
}
func hasService(name string) func(sample) bool {
	return func(s sample) bool { _, ok := s.Services[name]; return ok }
}
func hasHostCPU(s sample) bool     { return s.HostCPU != nil }
func hasConnections(s sample) bool { return s.Connections != nil }
func always(sample) bool           { return true }

func covered(w window, samples []sample, has func(sample) bool) bool {
	var times []time.Time
	for _, s := range samples {
		if w.contains(s.Time) && has(s) {
			times = append(times, s.Time)
		}
	}
	if len(times) == 0 {
		return false
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	previous := w.start
	for _, at := range times {
		if at.Sub(previous) > 5*time.Second {
			return false
		}
		previous = at
	}
	return w.end.Sub(previous) <= 5*time.Second
}
func allCovered(ws []window, samples []sample, has func(sample) bool) bool {
	for _, w := range ws {
		if !covered(w, samples, has) {
			return false
		}
	}
	return true
}
func coverage(records []loadrun.Record, samples []sample, names []string) bool {
	ws := windows(records)
	if !allCovered(ws, samples, always) {
		return false
	}
	for _, name := range names {
		if !allCovered(ws, samples, hasProcess(name)) {
			return false
		}
	}
	return true
}
