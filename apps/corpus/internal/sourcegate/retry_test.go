package sourcegate

import (
	"testing"
	"time"
)

func TestRetry_SourceTable(t *testing.T) {
	for _, status := range []int{200, 404, 418, 500, 504} {
		p := Retry{Backoffs: []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}}
		transient := status >= 500 || status == 200
		d, retry, _ := p.Next(status, transient, 0)
		if retry != transient || retry && d != 2*time.Second {
			t.Fatalf("status=%d delay=%s retry=%v", status, d, retry)
		}
		if _, again, _ := p.Next(status, transient, 0); again {
			t.Fatalf("second source retry status=%d", status)
		}
	}
}
func TestRetry_429WaitBudget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		after    time.Duration
		waits    []time.Duration
		deferred bool
	}{
		{"scheduled", 0, []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}, false},
		{"later Retry-After", 50 * time.Second, []time.Duration{50 * time.Second, 50 * time.Second, 50 * time.Second}, false},
		{"wait cap", 100 * time.Second, []time.Duration{100 * time.Second, 100 * time.Second}, false},
		{"does not fit", 201 * time.Second, nil, false},
		{"long gap", 601 * time.Second, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Retry{Backoffs: []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}}
			for _, want := range tc.waits {
				d, yes, gap := p.Next(429, false, tc.after)
				if !yes || gap || d != want {
					t.Fatalf("%s %v %v", d, yes, gap)
				}
			}
			if _, yes, gap := p.Next(429, false, tc.after); yes || gap != tc.deferred {
				t.Fatalf("retry=%v gap=%v", yes, gap)
			}
		})
	}
}
func TestDownloadTimeout(t *testing.T) {
	for _, tc := range []struct {
		size int64
		want time.Duration
	}{{20_000_000, 110 * time.Second}, {50_000_000, 230 * time.Second}, {-1, 230 * time.Second}, {0, 30 * time.Second}} {
		if got := DownloadTimeout(tc.size); got != tc.want {
			t.Fatalf("size=%d deadline=%s", tc.size, got)
		}
	}
}
func TestRetryAfter_Parse(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{{"", 0}, {"invalid", 0}, {"10", 10 * time.Second}, {"999999999999999999999", 601 * time.Second}, {now.Add(20 * time.Second).Format("Mon, 02 Jan 2006 15:04:05 GMT"), 20 * time.Second}} {
		if got := RetryAfter(tc.value, now); got != tc.want {
			t.Fatalf("value=%s delay=%s", tc.value, got)
		}
	}
}

func TestRetry_WaitCapIncludesTransientRetry(t *testing.T) {
	p := Retry{Backoffs: []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}}
	if d, retry, _ := p.Next(500, true, 0); !retry || d != 2*time.Second {
		t.Fatal(d, retry)
	}
	if d, retry, _ := p.Next(429, false, 100*time.Second); !retry || d != 100*time.Second {
		t.Fatal(d, retry)
	}
	if _, retry, _ := p.Next(429, false, 100*time.Second); retry {
		t.Fatal("waits exceeded 200 seconds")
	}
}
