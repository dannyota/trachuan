package password

import (
	"testing"
	"time"
)

func TestCounter_ExactAddressWindowAndBounds(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCounter(func() time.Time { return now })
	for range 10 {
		if !c.Allow("person@example.test") {
			t.Fatal("attempt refused early")
		}
	}
	if c.Allow("person@example.test") {
		t.Fatal("eleventh admitted")
	}
	if !c.Allow("Person@example.test") {
		t.Fatal("local part case lost")
	}
	now = now.Add(15 * time.Minute)
	if !c.Allow("person@example.test") {
		t.Fatal("window did not expire")
	}
	if !c.Allow("Person@example.test") {
		t.Fatal("second counter")
	}
	c.maxEntries = 2
	if c.Allow("other@example.test") {
		t.Fatal("counter map not bounded")
	}
	now = now.Add(15 * time.Minute)
	if !c.Allow("other@example.test") {
		t.Fatal("expired counters not reclaimed")
	}
}
