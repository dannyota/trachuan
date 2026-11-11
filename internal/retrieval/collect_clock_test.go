package retrieval

import (
	"testing"
	"time"
)

func TestCollect_RequestInstantAndAsOf(t *testing.T) {
	before := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	calls := 0
	e := &Engine{options: Options{Now: func() time.Time { calls++; return before.Add(time.Second) }}}
	if got := e.searchDate(Request{Instant: before}); got != "2099-01-01" || calls != 0 {
		t.Fatal("explicit instant resampled clock", got, calls)
	}
	if got := e.searchDate(Request{}); got != "2099-01-02" || calls != 1 {
		t.Fatal("zero instant did not use engine clock", got, calls)
	}
	asOf := &Date{Year: 2080, Month: time.May, Day: 3}
	if got := e.searchDate(Request{Instant: before, AsOf: asOf}); got != "2080-05-03" {
		t.Fatal("instant changed as_of", got)
	}
}
