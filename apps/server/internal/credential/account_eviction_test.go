package credential

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func evictAccount(t *testing.T, v *Verifier, id int64) {
	t.Helper()
	e, ok := any(v).(interface{ EvictAccount(int64) })
	if !ok {
		t.Fatal("missing account eviction")
	}
	e.EvictAccount(id)
}

func TestEvictAccount_WarmAndOtherAccount(t *testing.T) {
	v, s, _, token := fixture(t)
	if _, err := v.Verify(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	other := "trc_1111111111111111111111111111111111111111111"
	s.row.AccountID = 8
	if _, err := v.Verify(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	evictAccount(t, v, 7)
	s.err = pgx.ErrNoRows
	if _, err := v.Verify(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("account credential cached")
	}
	if _, err := v.Verify(context.Background(), other); err != nil {
		t.Fatal("other account evicted")
	}
}

func TestEvictAccount_InFlight(t *testing.T) {
	v, s, _, token := fixture(t)
	entered, resume := make(chan struct{}), make(chan struct{})
	s.lookup = func() { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { _, err := v.Verify(context.Background(), token); done <- err }()
	<-entered
	evictAccount(t, v, 7)
	close(resume)
	if err := <-done; !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale lookup accepted")
	}
	s.lookup = nil
	s.err = pgx.ErrNoRows
	if _, err := v.Verify(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale lookup cached")
	}
}

func TestEvictAccount_OtherAccountInFlight(t *testing.T) {
	v, s, _, token := fixture(t)
	entered, resume := make(chan struct{}), make(chan struct{})
	s.lookup = func() { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { _, err := v.Verify(context.Background(), token); done <- err }()
	<-entered
	v.EvictAccount(8)
	close(resume)
	if err := <-done; err != nil {
		t.Fatal("other account lookup rejected")
	}
	s.lookup = nil
	s.err = pgx.ErrNoRows
	if _, err := v.Verify(context.Background(), token); err != nil {
		t.Fatal("other account lookup not cached")
	}
}
