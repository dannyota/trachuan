package credential

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type fakeStore struct {
	mu               sync.Mutex
	row              store.GetCredentialRow
	err              error
	touchErr         error
	lookups, touches int
	lastHash         string
	lookup           func()
	touch            func()
}

func (s *fakeStore) GetCredential(_ context.Context, hash string) (store.GetCredentialRow, error) {
	s.mu.Lock()
	s.lookups++
	s.lastHash = hash
	row, err, lookup := s.row, s.err, s.lookup
	s.mu.Unlock()
	if lookup != nil {
		lookup()
	}
	return row, err
}

func (s *fakeStore) TouchPersonalToken(context.Context, string) error {
	s.mu.Lock()
	s.touches++
	err, touch := s.touchErr, s.touch
	s.mu.Unlock()
	if touch != nil {
		touch()
	}
	return err
}

func fixture(t *testing.T) (*Verifier, *fakeStore, *time.Time, string) {
	t.Helper()
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	s := &fakeStore{row: store.GetCredentialRow{AccountID: 7, AccountAuthEpoch: 2, TokenAuthEpoch: 2, ExpiresAt: now.Add(time.Hour)}}
	v := &Verifier{Store: s, Clock: func() time.Time { return now }, CacheTTL: 10 * time.Second, Audience: "https://example.invalid/mcp"}
	plain, _, err := Issue(bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return v, s, &now, plain
}

func TestVerify_RejectsRevokedExpiredBlockedOldEpochUnknown(t *testing.T) {
	for _, name := range []string{"revoked", "expired", "blocked", "old epoch", "future epoch", "unknown"} {
		t.Run(name, func(t *testing.T) {
			v, s, now, plain := fixture(t)
			switch name {
			case "revoked":
				s.row.RevokedAt = now
			case "expired":
				s.row.ExpiresAt = *now
			case "blocked":
				s.row.BlockedAt = now
			case "old epoch":
				s.row.TokenAuthEpoch--
			case "future epoch":
				s.row.TokenAuthEpoch++
			case "unknown":
				s.err = pgx.ErrNoRows
			}
			for range 2 {
				p, err := v.Verify(context.Background(), plain)
				if !errors.Is(err, ErrUnauthorized) || p != (Principal{}) {
					t.Fatal("invalid credential accepted")
				}
			}
			if s.lookups != 2 || s.touches != 0 {
				t.Fatal("negative result cached or touched")
			}
		})
	}
}

func TestVerify_ValidPrincipalAndHashLookup(t *testing.T) {
	v, s, _, plain := fixture(t)
	p, err := v.Verify(context.Background(), plain)
	if err != nil || p != (Principal{AccountID: 7, Scope: "mcp:read", Audience: v.Audience}) {
		t.Fatalf("principal mismatch: %v", err)
	}
	_, hash, _ := Issue(bytes.NewReader(make([]byte, 32)))
	if s.lastHash != hash || s.touches != 1 {
		t.Fatal("lookup or touch mismatch")
	}
}

func TestVerify_MalformedNeverQueriesStore(t *testing.T) {
	v, s, _, plain := fixture(t)
	for _, token := range []string{"", plain[1:], plain + "=", plain + "x", "TRC_" + plain[4:], plain[:46], "trc_" + strings.Repeat("!", 43), plain + "\n"} {
		if _, err := v.Verify(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("malformed token accepted")
		}
	}
	if s.lookups != 0 {
		t.Fatal("malformed input queried store")
	}
}

func TestVerify_AuthDownFailsClosed(t *testing.T) {
	v, s, _, plain := fixture(t)
	s.err = errors.New("sensitive query text")
	for range 2 {
		p, err := v.Verify(context.Background(), plain)
		if !errors.Is(err, ErrUnavailable) || p != (Principal{}) || strings.Contains(err.Error(), "sensitive") {
			t.Fatal("database failure must fail closed with a safe error")
		}
	}
	if s.lookups != 2 {
		t.Fatal("database error cached")
	}
	s.err = nil
	if _, err := v.Verify(context.Background(), plain); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_TouchOncePerMinuteDespiteCacheExpiry(t *testing.T) {
	v, s, now, plain := fixture(t)
	for _, tc := range []struct {
		elapsed time.Duration
		touches int
	}{
		{0, 1},
		{10 * time.Second, 1},
		{59 * time.Second, 1},
		{time.Minute, 1},
		{61 * time.Second, 1},
		{69 * time.Second, 2},
	} {
		*now = time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC).Add(tc.elapsed)
		if _, err := v.Verify(context.Background(), plain); err != nil {
			t.Fatal(err)
		}
		if s.touches != tc.touches {
			t.Fatalf("at %v touches=%d, want %d", tc.elapsed, s.touches, tc.touches)
		}
	}
}

func TestVerify_ConcurrentSafe(t *testing.T) {
	v, s, _, plain := fixture(t)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if _, err := v.Verify(context.Background(), plain); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if s.touches != 1 {
		t.Fatalf("touches=%d, want 1", s.touches)
	}
}

func TestVerify_CacheAndTouchStateBoundedOldestFirst(t *testing.T) {
	v, s, now, first := fixture(t)
	ctx := context.Background()
	if _, err := v.Verify(ctx, first); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10000; i++ {
		plain := fmt.Sprintf("trc_%043d", i)
		if _, err := v.Verify(ctx, plain); err != nil {
			t.Fatal(err)
		}
		if i == 9999 {
			// Reading the oldest entry must not turn FIFO eviction into LRU.
			if _, err := v.Verify(ctx, first); err != nil {
				t.Fatal(err)
			}
		}
	}
	before := s.lookups
	if _, err := v.Verify(ctx, first); err != nil {
		t.Fatal(err)
	}
	if s.lookups != before+1 {
		t.Fatal("oldest entry was not evicted")
	}
	if s.touches != 10000 {
		t.Fatalf("touches=%d, want bounded touch admission", s.touches)
	}
	if len(v.entries) != 10000 || len(v.touches) != 10000 || v.used.Len() != 10000 || v.oldest.Len() != 10000 {
		t.Fatal("cache or touch state exceeded its bound")
	}
	*now = now.Add(time.Minute)
	if _, err := v.Verify(ctx, first); err != nil {
		t.Fatal(err)
	}
	if s.touches != 10001 || len(v.touches) != 1 || v.used.Len() != 1 {
		t.Fatal("expired touch state was not reclaimed")
	}
}

func TestVerify_SlowLookupDoesNotExtendCacheWindow(t *testing.T) {
	v, s, now, plain := fixture(t)
	s.lookup = func() { *now = now.Add(11 * time.Second) }
	if p, err := v.Verify(context.Background(), plain); !errors.Is(err, ErrUnavailable) || p != (Principal{}) {
		t.Fatal("slow lookup must fail unavailable without a principal")
	}
	if len(v.entries) != 0 {
		t.Fatal("stale lookup was cached")
	}
	s.err = pgx.ErrNoRows
	if _, err := v.Verify(context.Background(), plain); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("slow lookup extended stale authorization")
	}
}

func TestVerify_ExpiryDuringStoreWork(t *testing.T) {
	for _, phase := range []string{"lookup", "touch"} {
		t.Run(phase, func(t *testing.T) {
			v, s, now, plain := fixture(t)
			s.row.ExpiresAt = now.Add(5 * time.Second)
			advance := func() { *now = s.row.ExpiresAt }
			if phase == "lookup" {
				s.lookup = advance
			} else {
				s.touch = advance
			}
			if _, err := v.Verify(context.Background(), plain); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("expired credential accepted after store work")
			}
			if len(v.entries) != 0 {
				t.Fatal("expired credential was cached")
			}
		})
	}
}
