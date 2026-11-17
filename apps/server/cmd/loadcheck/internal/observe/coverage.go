package observe

import (
	"sort"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

func coverage(records []loadrun.Record, samples []sample, names []string) bool {
	for _, phase := range []string{"A", "B", "C", "D"} {
		start, end := bounds(records, phase)
		if start.IsZero() {
			continue
		}
		checks := append([]string{""}, names...)
		for _, name := range checks {
			var times []time.Time
			for _, s := range samples {
				if s.Time.Before(start) || s.Time.After(end) {
					continue
				}
				if name != "" && len(s.Processes[name].Processes) == 0 {
					continue
				}
				times = append(times, s.Time)
			}
			sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
			previous := start
			if len(times) == 0 {
				return false
			}
			for _, at := range times {
				if at.Sub(previous) > 5*time.Second {
					return false
				}
				previous = at
			}
			if end.Sub(previous) > 5*time.Second {
				return false
			}
		}
	}
	return true
}
