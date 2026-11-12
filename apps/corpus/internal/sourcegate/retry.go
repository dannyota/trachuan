package sourcegate

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryAfter parses a source wait without allowing integer overflow.
func RetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value != "" && strings.Trim(value, "0123456789") == "" {
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil || n > 600 {
			return 601 * time.Second
		}
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// Retry bounds transient retries separately from the source's load signals.
type Retry struct {
	Backoffs  []time.Duration
	WaitLimit time.Duration
	transient int
	limited   int
	waited    time.Duration
}

// Next returns a bounded retry wait, or reports that Retry-After is a gap.
func (r *Retry) Next(status int, transient bool, after time.Duration) (delay time.Duration, retry, deferred bool) {
	if status == 429 {
		if after > 10*time.Minute {
			return 0, false, true
		}
		if r.limited >= min(3, len(r.Backoffs)) {
			return 0, false, false
		}
		delay = max(r.Backoffs[r.limited], after)
		limit := r.WaitLimit
		if limit <= 0 {
			limit = 200 * time.Second
		}
		if delay > limit-r.waited {
			return 0, false, false
		}
		r.limited++
		r.waited += delay
		return delay, true, false
	}
	if !transient || r.transient >= 1 || len(r.Backoffs) == 0 {
		return 0, false, false
	}
	delay = r.Backoffs[0]
	limit := r.WaitLimit
	if limit <= 0 {
		limit = 200 * time.Second
	}
	if delay > limit-r.waited {
		return 0, false, false
	}
	r.transient++
	r.waited += delay
	return delay, true, false
}

// DownloadTimeout allows at least 250 kB/s, with time for connection setup.
func DownloadTimeout(size int64) time.Duration {
	if size < 0 || size > 50_000_000 {
		size = 50_000_000
	}
	return 30*time.Second + time.Duration(size)*time.Second/250_000
}
