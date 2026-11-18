package observe

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type process struct {
	PID         int     `json:"pid"`
	Start       string  `json:"start"`
	RSS         uint64  `json:"rss_bytes"`
	LifetimeCPU float64 `json:"lifetime_cpu_percent"`
}
type tracker struct{ previous map[string]process }

func newTracker() *tracker { return &tracker{previous: map[string]process{}} }
func (t *tracker) observe(name string, processes []process) bool {
	var oldest process
	var oldestTime time.Time
	for _, p := range processes {
		started, err := time.Parse(time.ANSIC, p.Start)
		if err != nil {
			continue
		}
		if oldestTime.IsZero() || started.Before(oldestTime) || (started.Equal(oldestTime) && p.PID < oldest.PID) {
			oldest, oldestTime = p, started
		}
	}
	if oldestTime.IsZero() {
		return false
	}
	previous, exists := t.previous[name]
	t.previous[name] = oldest
	return exists && (previous.PID != oldest.PID || previous.Start != oldest.Start)
}
func readProcesses(ctx context.Context, expressions map[string]*regexp.Regexp) (map[string][]process, float64, error) {
	output, err := exec.CommandContext(ctx, "env", "LC_ALL=C", "ps", "-axo", "pid=,lstart=,rss=,pcpu=,comm=").Output()
	if err != nil {
		return nil, 0, errors.New("process sampling failed")
	}
	return parseProcesses(bytes.NewReader(output), expressions, os.Getpid())
}
func parseProcesses(reader io.Reader, expressions map[string]*regexp.Regexp, selfPID int) (map[string][]process, float64, error) {
	result := map[string][]process{}
	for name := range expressions {
		result[name] = nil
	}
	total := 0.0
	scanner := bufio.NewScanner(reader)
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
		start := strings.Join(fields[1:6], " ")
		matched := pid == selfPID
		for _, regex := range expressions {
			if regex.MatchString(fields[8]) {
				matched = true
				break
			}
		}
		if matched {
			if _, err := time.Parse(time.ANSIC, start); err != nil {
				return nil, 0, errors.New("invalid process start time")
			}
		}
		item := process{PID: pid, Start: start, RSS: rss * 1024, LifetimeCPU: cpu}
		if pid == selfPID {
			result["loadcheck"] = append(result["loadcheck"], item)
		}
		for name, regex := range expressions {
			if regex.MatchString(fields[8]) {
				result[name] = append(result[name], item)
			}
		}
	}
	if scanner.Err() != nil {
		return nil, 0, errors.New("process sampling failed")
	}
	return result, total, nil
}
