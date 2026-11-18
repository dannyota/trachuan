package observe

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type serviceSample struct {
	CPU     *float64 `json:"cpu_percent"`
	Current uint64   `json:"memory_current_bytes"`
	Peak    uint64   `json:"memory_peak_bytes"`
	Restart bool     `json:"restart"`
}
type serviceState struct {
	usec    uint64
	at      time.Time
	present bool
}
type cgroupReader struct {
	root     string
	names    []string
	previous map[string]serviceState
}

func newCgroupReader(root string, names []string) *cgroupReader {
	return &cgroupReader{root: root, names: names, previous: map[string]serviceState{}}
}
func (c *cgroupReader) read(at time.Time) (map[string]serviceSample, error) {
	result := map[string]serviceSample{}
	for _, name := range c.names {
		dir := filepath.Join(c.root, "trachuan-"+name+".service")
		usec, current, peak, found, err := readService(dir)
		if err != nil {
			return nil, err
		}
		previous, seen := c.previous[name]
		if !found {
			c.previous[name] = serviceState{}
			continue
		}
		v := serviceSample{Current: current, Peak: peak, Restart: seen && (!previous.present || usec < previous.usec)}
		if seen && previous.present && !v.Restart && at.After(previous.at) {
			cpu := 100 * float64(usec-previous.usec) / float64(at.Sub(previous.at).Microseconds())
			v.CPU = &cpu
		}
		c.previous[name] = serviceState{usec: usec, at: at, present: true}
		result[name] = v
	}
	return result, nil
}
func readService(dir string) (usec, current, peak uint64, found bool, err error) {
	stat, err := os.ReadFile(filepath.Join(dir, "cpu.stat"))
	if serviceGone(err) {
		return 0, 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("read cgroup: %w", err)
	}
	usec, err = parseUsage(string(stat))
	if err != nil {
		return 0, 0, 0, false, err
	}
	var values [2]uint64
	for i, file := range []string{"memory.current", "memory.peak"} {
		data, e := os.ReadFile(filepath.Join(dir, file))
		if serviceGone(e) {
			return 0, 0, 0, false, nil
		}
		if e != nil {
			return 0, 0, 0, false, fmt.Errorf("read cgroup: %w", e)
		}
		if values[i], e = strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); e != nil {
			return 0, 0, 0, false, errors.New("invalid cgroup memory value")
		}
	}
	return usec, values[0], values[1], true, nil
}
func parseUsage(stat string) (uint64, error) {
	for _, line := range strings.Split(stat, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			n, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				break
			}
			return n, nil
		}
	}
	return 0, errors.New("invalid cgroup cpu.stat")
}

func serviceGone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENODEV)
}
