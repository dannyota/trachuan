package observe

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/release/waitlog"
)

type logLine struct {
	Msg  string `json:"msg"`
	Time string `json:"time"`
	Path string `json:"path"`
}

func renders(reader io.Reader, start, end time.Time) (map[string]int, error) {
	result := map[string]int{}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var line logLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Msg != "summary render" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, line.Time)
		if err != nil {
			return nil, errors.New("summary render has invalid time")
		}
		_, offset := at.Zone()
		if offset != 0 {
			return nil, errors.New("summary render has invalid time")
		}
		if at.Before(start) || at.After(end) {
			continue
		}
		if !strings.HasPrefix(line.Path, "/vn/documents/") || !strings.HasSuffix(line.Path, "/summary") {
			return nil, errors.New("invalid summary render path")
		}
		key, err := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(line.Path, "/vn/documents/"), "/summary"))
		if err != nil || key == "" {
			return nil, errors.New("invalid summary render path")
		}
		result[key]++
	}
	if scanner.Err() != nil {
		return nil, errors.New("read web log failed")
	}
	return result, nil
}
func poolP99(reader io.Reader, start, end time.Time) (float64, error) {
	return waitlog.P99(reader, start, end)
}
func latencyGate(p95, p99, baseline, limit95, limit99 float64) bool {
	return p95 <= limit95 && p99 <= limit99 && p95 <= baseline*1.25
}
