package password

import (
	"testing"
	"time"
)

func TestCounter_ExactAddressWindowAndBounds(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCounter(func() time.Time { return now })
	for range 10 {
		if !allowFailure(c, "person@example.test") {
			t.Fatal("attempt refused early")
		}
	}
	if allowFailure(c, "person@example.test") {
		t.Fatal("eleventh admitted")
	}
	if !allowFailure(c, "Person@example.test") {
		t.Fatal("local part case lost")
	}
	now = now.Add(15 * time.Minute)
	if !allowFailure(c, "person@example.test") {
		t.Fatal("window did not expire")
	}
	if !allowFailure(c, "Person@example.test") {
		t.Fatal("second counter")
	}
	c.maxEntries = 2
	if allowFailure(c, "other@example.test") {
		t.Fatal("counter map not bounded")
	}
	now = now.Add(15 * time.Minute)
	if !allowFailure(c, "other@example.test") {
		t.Fatal("expired counters not reclaimed")
	}
}

func allowFailure(c *Counter, address string) bool {
	finish, ok := c.Reserve(address)
	if ok {
		finish(false)
	}
	return ok
}

func TestCounter_SuccessReleasesOnlyItsReservation(t *testing.T) {
	c := NewCounter(time.Now)
	for range 9 {
		if !allowFailure(c, "person@example.test") {
			t.Fatal("failure refused early")
		}
	}
	finish, ok := c.Reserve("person@example.test")
	if !ok {
		t.Fatal("last slot refused")
	}
	if _, ok = c.Reserve("person@example.test"); ok {
		t.Fatal("in-flight reservation bypassed")
	}
	finish(true)
	finish(true)
	if !allowFailure(c, "person@example.test") {
		t.Fatal("successful reservation not released")
	}
	if allowFailure(c, "person@example.test") {
		t.Fatal("success erased earlier failures or released twice")
	}
}

func TestCounter_InFlightReservationsSurviveExpiry(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCounter(func() time.Time { return now })
	c.maxEntries = 1
	var finishes []func(bool)
	for range 10 {
		finish, ok := c.Reserve("person@example.test")
		if !ok {
			t.Fatal("reservation refused early")
		}
		finishes = append(finishes, finish)
	}
	now = now.Add(15 * time.Minute)
	if _, ok := c.Reserve("other@example.test"); ok {
		t.Fatal("active entry evicted")
	}
	if _, ok := c.Reserve("person@example.test"); ok {
		t.Fatal("active reservations lost at window boundary")
	}
	finishes[0](true)
	if !allowFailure(c, "person@example.test") {
		t.Fatal("late success did not release its reservation")
	}
	for _, finish := range finishes[1:] {
		finish(true)
	}
	for range 9 {
		if !allowFailure(c, "person@example.test") {
			t.Fatal("late success consumed a failure slot")
		}
	}
	if allowFailure(c, "person@example.test") {
		t.Fatal("late success erased a new failure")
	}
}

func TestCounter_SuccessReclaimsCapacity(t *testing.T) {
	c := NewCounter(time.Now)
	c.maxEntries = 1
	finish, ok := c.Reserve("person@example.test")
	if !ok {
		t.Fatal("first reservation refused")
	}
	finish(true)
	if !allowFailure(c, "other@example.test") {
		t.Fatal("success retained an empty entry")
	}
}
