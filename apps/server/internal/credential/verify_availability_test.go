package credential

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestVerify_TouchFailureFailsClosedAndIsNotCached(t *testing.T) {
	v, s, _, plain := fixture(t)
	hash := tokenHash(plain)
	s.touchErr = errors.New("sensitive query " + plain + " " + hash)
	p, err := v.Verify(context.Background(), plain)
	if !errors.Is(err, ErrUnavailable) || p != (Principal{}) {
		t.Fatal("touch failure must fail unavailable without a principal")
	}
	for _, text := range []string{"sensitive", "query", plain, hash} {
		if strings.Contains(err.Error(), text) {
			t.Fatal("touch failure disclosed database details")
		}
	}
	if len(v.entries) != 0 {
		t.Fatal("failed touch populated the credential cache")
	}
	s.err = errors.New("store unavailable")
	if _, err := v.Verify(context.Background(), plain); !errors.Is(err, ErrUnavailable) || s.lookups != 2 {
		t.Fatal("failed touch bypassed the next database lookup")
	}
}

func TestVerify_CachedHitDoesNotQueryStore(t *testing.T) {
	v, s, now, plain := fixture(t)
	if _, err := v.Verify(context.Background(), plain); err != nil {
		t.Fatal(err)
	}
	s.err, s.touchErr = errors.New("lookup unavailable"), errors.New("touch unavailable")
	*now = now.Add(10*time.Second - time.Nanosecond)
	if _, err := v.Verify(context.Background(), plain); err != nil {
		t.Fatal("cached credential must not require store availability")
	}
	if s.lookups != 1 || s.touches != 1 {
		t.Fatal("cached verification queried the store")
	}
	*now = now.Add(time.Nanosecond)
	if _, err := v.Verify(context.Background(), plain); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cache deadline must restore the store availability requirement")
	}
}

func TestVerify_RevokeDuringBlockedStoreWorkFailsUnavailable(t *testing.T) {
	for _, phase := range []string{"lookup", "touch"} {
		t.Run(phase, func(t *testing.T) {
			v, s, now, plain := fixture(t)
			entered, resume := make(chan struct{}), make(chan struct{})
			block := func() { close(entered); <-resume }
			if phase == "lookup" {
				s.lookup = block
			} else {
				s.touch = block
			}
			done := make(chan int, 1)
			go func() { done <- response(v, []string{"Bearer " + plain}).Code }()
			<-entered
			s.mu.Lock()
			revokedAt := now.Add(time.Second)
			s.row.RevokedAt = &revokedAt
			s.mu.Unlock()
			*now = now.Add(10*time.Second + time.Nanosecond)
			close(resume)
			if status := <-done; status != http.StatusServiceUnavailable {
				t.Fatalf("stale credential returned HTTP %d, want 503", status)
			}
			if len(v.entries) != 0 {
				t.Fatal("revoked credential from stale store work was cached")
			}
			s.lookup, s.touch = nil, nil
			if _, err := v.Verify(context.Background(), plain); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("next lookup did not observe revocation")
			}
		})
	}
}

func TestVerify_StoreWorkTTLBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		elapsed time.Duration
		stale   bool
	}{
		{"at deadline", 10 * time.Second, 10 * time.Second, false},
		{"past deadline", 10 * time.Second, 10*time.Second + time.Nanosecond, true},
		{"short ttl", time.Second, time.Second + time.Nanosecond, true},
		{"default ttl", 0, 11 * time.Second, true},
		{"clamped ttl", time.Hour, 11 * time.Second, true},
		{"cache disabled", -1, time.Nanosecond, false},
		{"cache disabled stale", -1, 11 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, s, now, plain := fixture(t)
			v.CacheTTL = tc.ttl
			s.touch = func() { *now = now.Add(tc.elapsed) }
			p, err := v.Verify(context.Background(), plain)
			if tc.stale {
				if !errors.Is(err, ErrUnavailable) || p != (Principal{}) {
					t.Fatal("stale store work returned a principal")
				}
			} else if err != nil || p.AccountID != 7 {
				t.Fatal("store work within its freshness window was denied")
			}
			if len(v.entries) != 0 {
				t.Fatal("result was cached past its deadline or with caching disabled")
			}
		})
	}
}
