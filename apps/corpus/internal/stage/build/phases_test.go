package build

import (
	"testing"
	"time"
)

func TestPhases_RecordsElapsedMilliseconds(t *testing.T) {
	p := phases{}
	start := time.Now()
	p.observe("plan", start)
	if value, ok := p["plan"]; !ok || value < 0 {
		t.Fatalf("plan duration = %d, present = %v", value, ok)
	}
	p.observe("load", start.Add(-5*time.Millisecond))
	if p["load"] < 5 || p["load"] < p["plan"] {
		t.Fatalf("durations = %v", p)
	}
}
