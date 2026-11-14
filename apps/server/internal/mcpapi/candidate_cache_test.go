package mcpapi

import (
	"testing"
	"time"
)

func TestSearchCache_ResponseBudget(t *testing.T) {
	c := NewCaches(time.Now)
	c.ActivateRelease("20990101", 1)
	if got := c.Scope("20990101", 1).Search.MaxValueBytes(); got != 62_000_000 {
		t.Fatalf("response value cap = %d, want 62000000", got)
	}
}

func TestCandidateCache_BudgetAgeAndActivation(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCaches(func() time.Time { return now })
	c.ActivateRelease("20990101", 1)
	old := c.Scope("20990101", 1).Candidates
	if got := old.MaxValueBytes(); got != 2_000_000 {
		t.Fatalf("candidate value cap = %d, want 2000000", got)
	}
	key := [32]byte{1}
	old.Put("20990101", key, []byte("candidates"))
	now = now.Add(time.Hour - time.Nanosecond)
	if _, ok := old.Get("20990101", key); !ok {
		t.Fatal("expired early")
	}
	now = now.Add(time.Nanosecond)
	if _, ok := old.Get("20990101", key); ok {
		t.Fatal("retained expired value")
	}
	old.Put("20990101", key, []byte("candidates"))
	c.ActivateRelease("20990102", 2)
	if c.Candidates.Stats().Bytes != 0 {
		t.Fatal("activation retained candidates")
	}
	c.ActivateRelease("20990101", 3)
	fresh := c.Scope("20990101", 3).Candidates
	fresh.Put("20990101", key, []byte("fresh"))
	if _, ok := old.Get("20990101", key); ok {
		t.Fatal("old handle read rollback")
	}
	old.Put("20990101", key, []byte("old"))
	if got, ok := fresh.Get("20990101", key); !ok || string(got) != "fresh" {
		t.Fatal("old handle overwrote rollback")
	}
}
