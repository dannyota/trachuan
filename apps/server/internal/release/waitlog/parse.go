package waitlog

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"math"
	"time"
)

type logLine struct {
	Msg    string    `json:"msg"`
	Time   string    `json:"time"`
	Bounds []float64 `json:"bucket_bounds_ms"`
	Counts []int     `json:"bucket_counts"`
	Count  int       `json:"count"`
}

func P99(reader io.Reader, start, end time.Time) (float64, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	var bounds []float64
	var counts []int
	total := 0
	for scanner.Scan() {
		var line logLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Msg != "reader pool wait" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, line.Time)
		if err != nil {
			return 0, errors.New("reader pool wait has invalid time")
		}
		if at.Before(start) || at.After(end.Add(time.Minute)) {
			continue
		}
		if len(line.Bounds) == 0 || len(line.Counts) != len(line.Bounds)+1 {
			return 0, errors.New("invalid reader wait buckets")
		}
		if bounds == nil {
			bounds = line.Bounds
			counts = make([]int, len(line.Counts))
		} else {
			if len(bounds) != len(line.Bounds) {
				return 0, errors.New("inconsistent reader wait buckets")
			}
			for i := range bounds {
				if bounds[i] != line.Bounds[i] {
					return 0, errors.New("inconsistent reader wait buckets")
				}
			}
		}
		subtotal := 0
		for i, n := range line.Counts {
			if n < 0 {
				return 0, errors.New("invalid reader wait buckets")
			}
			counts[i] += n
			subtotal += n
		}
		if subtotal != line.Count {
			return 0, errors.New("invalid reader wait count")
		}
		total += subtotal
	}
	if scanner.Err() != nil {
		return 0, errors.New("read server log failed")
	}
	if total == 0 {
		return 0, errors.New("no reader wait samples")
	}
	rank := (99*total + 99) / 100
	n := 0
	for i, count := range counts {
		n += count
		if n >= rank {
			if i == len(bounds) {
				return math.Inf(1), nil
			}
			return bounds[i], nil
		}
	}
	return 0, errors.New("invalid reader wait count")
}
