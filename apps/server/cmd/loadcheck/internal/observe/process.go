package observe

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type process struct {
	PID   int     `json:"pid"`
	Start string  `json:"start"`
	RSS   uint64  `json:"rss_bytes"`
	CPU   float64 `json:"cpu_percent"`
}
type tracker struct{ previous map[string]string }

func newTracker() *tracker { return &tracker{previous: map[string]string{}} }
func (t *tracker) observe(name string, processes []process) bool {
	ids := make([]string, 0, len(processes))
	for _, p := range processes {
		ids = append(ids, strconv.Itoa(p.PID)+":"+p.Start)
	}
	sort.Strings(ids)
	current := strings.Join(ids, ",")
	previous, exists := t.previous[name]
	if current != "" {
		t.previous[name] = current
	}
	return exists && current != "" && previous != current
}
func readProcesses(ctx context.Context, expressions map[string]*regexp.Regexp) (map[string][]process, float64, error) {
	output, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,lstart=,rss=,pcpu=,comm=").Output()
	if err != nil {
		return nil, 0, errors.New("process sampling failed")
	}
	result := map[string][]process{}
	total := 0.0
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 9 {
			continue
		}
		pid, e1 := strconv.Atoi(fields[0])
		rss, e2 := strconv.ParseUint(fields[6], 10, 64)
		cpu, e3 := strconv.ParseFloat(fields[7], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		total += cpu
		if pid == os.Getpid() {
			result["loadcheck"] = append(result["loadcheck"], process{PID: pid, Start: strings.Join(fields[1:6], " "), RSS: rss * 1024, CPU: cpu})
		}
		for name, regex := range expressions {
			if regex.MatchString(fields[8]) {
				result[name] = append(result[name], process{PID: pid, Start: strings.Join(fields[1:6], " "), RSS: rss * 1024, CPU: cpu})
			}
		}
	}
	if scanner.Err() != nil {
		return nil, 0, errors.New("process sampling failed")
	}
	return result, total, nil
}
