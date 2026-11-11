package credential

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func evictForTest(t *testing.T, v *Verifier, hash string) {
	t.Helper()
	e, ok := any(v).(interface{ Evict(string) })
	if !ok {
		t.Fatal("verifier does not support eviction")
	}
	e.Evict(hash)
}

func TestEvict_WarmAndUnrelated(t *testing.T) {
	v, s, _, first := fixture(t)
	other := "trc_" + "1111111111111111111111111111111111111111111"
	for _, token := range []string{first, other} {
		if _, err := v.Verify(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	evictForTest(t, v, tokenHash(first))
	s.err = pgx.ErrNoRows
	if _, err := v.Verify(context.Background(), first); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("evicted token accepted")
	}
	if _, err := v.Verify(context.Background(), other); err != nil {
		t.Fatal("unrelated token evicted")
	}
}

func TestEvict_LookupInFlight(t *testing.T) {
	v, s, _, token := fixture(t)
	entered, resume := make(chan struct{}), make(chan struct{})
	s.lookup = func() { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { _, err := v.Verify(context.Background(), token); done <- err }()
	<-entered
	evictForTest(t, v, tokenHash(token))
	close(resume)
	if err := <-done; !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked in-flight lookup authorized")
	}
	s.lookup = nil
	s.err = pgx.ErrNoRows
	if _, err := v.Verify(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("in-flight lookup repopulated cache")
	}
}
