package loadrun

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

type queries struct {
	mu     sync.Mutex
	values []string
	pos    int
}

func newQueries(values []string) (*queries, error) {
	seen := map[string]bool{}
	for _, v := range values {
		if v == "" || seen[v] {
			return nil, errors.New("queries must be nonempty and distinct")
		}
		seen[v] = true
	}
	if len(values) == 0 {
		return nil, errors.New("queries are empty")
	}
	return &queries{values: values}, nil
}
func (q *queries) next() (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pos == len(q.values) {
		return "", errors.New("queries exhausted")
	}
	v := q.values[q.pos]
	q.pos++
	return v, nil
}
func schedule(ctx context.Context, duration time.Duration, rate int, action func(int)) {
	var group sync.WaitGroup
	start := time.Now()
	for i := 0; ; i++ {
		due := start.Add(time.Duration(int64(time.Second) * int64(i) / int64(rate)))
		if duration > 0 && !due.Before(start.Add(duration)) {
			break
		}
		timer := time.NewTimer(time.Until(due))
		select {
		case <-ctx.Done():
			timer.Stop()
			group.Wait()
			return
		case <-timer.C:
		}
		group.Add(1)
		go func(n int) { defer group.Done(); action(n) }(i)
	}
	group.Wait()
}
func retryBusy(attempt func() (bool, time.Duration), sleep func(time.Duration)) int {
	retries := 0
	for {
		busy, retryAfter := attempt()
		if !busy || retries == 3 {
			return retries
		}
		delay := time.Second << retries
		if retryAfter > delay {
			delay = retryAfter
		}
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
		sleep(delay)
		retries++
	}
}
func abortRequest(i int, seed uint64) bool {
	x := uint64(i/20) + seed + 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	return i%20 == int(x%20)
}
func address(i, n int) string { return fmt.Sprintf("2001:db8::%x", i%n+1) }
func percentile(values []float64, p int) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	return copyValues[int(math.Ceil(float64(len(copyValues))*float64(p)/100))-1]
}
