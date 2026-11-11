package credential

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

type oauthStore struct {
	*fakeStore
	oauth                      store.GetTokenRow
	oauthErr                   error
	oauthTouchErr              error
	oauthLookups, oauthTouches int
}

func (s *oauthStore) GetToken(ctx context.Context, hash string) (store.GetTokenRow, error) {
	s.oauthLookups++
	if hash != tokenHash("tra_"+strings.Repeat("A", 43)) {
		return store.GetTokenRow{}, errors.New("wrong hash")
	}
	if _, ok := ctx.Deadline(); !ok {
		return store.GetTokenRow{}, errors.New("missing deadline")
	}
	row := s.oauth
	if s.lookup != nil {
		s.lookup()
	}
	return row, s.oauthErr
}
func (s *oauthStore) TouchOAuthGrant(context.Context, int64) error {
	s.oauthTouches++
	return s.oauthTouchErr
}
func oauthFixture(t *testing.T) (*Verifier, *oauthStore, *time.Time, string) {
	t.Helper()
	v, s, now, _ := fixture(t)
	db := &oauthStore{fakeStore: s, oauth: store.GetTokenRow{Kind: "access", AccountID: 7, GrantID: 9, Resource: v.Audience, Scope: "mcp:read", AuthEpoch: 2, AccountAuthEpoch: 2, ExpiresAt: now.Add(time.Hour), FamilyExpiresAt: now.Add(24 * time.Hour)}}
	v.Store = db
	return v, db, now, "tra_" + strings.Repeat("A", 43)
}
func TestBearer_ResourceEpochBlockAndCache(t *testing.T) {
	for _, name := range []string{"valid", "resource", "epoch", "future epoch", "blocked", "deleted", "grant revoked", "token revoked", "expired", "family expired", "refresh kind", "scope", "database", "touch"} {
		t.Run(name, func(t *testing.T) {
			v, s, now, raw := oauthFixture(t)
			want := error(ErrUnauthorized)
			switch name {
			case "valid":
				want = nil
			case "resource":
				s.oauth.Resource = "https://other.invalid/mcp"
			case "epoch":
				s.oauth.AuthEpoch--
			case "future epoch":
				s.oauth.AuthEpoch++
			case "blocked":
				s.oauth.BlockedAt = now
			case "deleted":
				s.oauth.DeleteRequestedAt = now
			case "grant revoked":
				s.oauth.GrantRevokedAt = now
			case "token revoked":
				s.oauth.RevokedAt = now
			case "expired":
				s.oauth.ExpiresAt = *now
			case "family expired":
				s.oauth.FamilyExpiresAt = *now
			case "refresh kind":
				s.oauth.Kind = "refresh"
			case "scope":
				s.oauth.Scope = "other" // middleware must issue insufficient_scope.
			case "database":
				s.oauthErr = errors.New("private database failure")
				want = ErrUnavailable
			case "touch":
				s.oauthTouchErr = errors.New("private database failure")
				want = ErrUnavailable
			}
			if name == "scope" {
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/mcp", nil)
				r.Header.Set("Authorization", "Bearer "+raw)
				Middleware(v, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("scope accepted") })).ServeHTTP(w, r)
				if w.Code != 403 || !strings.Contains(w.Header().Get("WWW-Authenticate"), `error="insufficient_scope"`) {
					t.Fatalf("scope status %d", w.Code)
				}
				return
			}
			_, err := v.Verify(t.Context(), raw)
			if !errors.Is(err, want) {
				t.Fatalf("error %v, want %v", err, want)
			}
			if name != "valid" {
				return
			}
			s.oauth.BlockedAt = now
			if _, err = v.Verify(t.Context(), raw); err != nil {
				t.Fatal("warm cache unexpectedly missed")
			}
			*now = now.Add(10 * time.Second)
			if _, err = v.Verify(t.Context(), raw); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("cache survived ten seconds")
			}
		})
	}
}
func TestBearer_OAuthAndPersonalTokensShareChecks(t *testing.T) {
	v, s, _, raw := oauthFixture(t)
	personal := "trc_" + strings.Repeat("A", 43)
	for _, token := range []string{raw, personal} {
		for range 2 {
			if _, err := v.Verify(t.Context(), token); err != nil {
				t.Fatal(err)
			}
		}
	}
	if s.oauthLookups != 1 || s.lookups != 1 {
		t.Fatal("prefix path or cache mismatch")
	}
	if _, err := v.Verify(t.Context(), "trr_"+raw[4:]); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("refresh authorizes MCP")
	}
}
func TestBearer_OAuthEvictionFences(t *testing.T) {
	for _, kind := range []string{"credential", "account", "grant"} {
		for _, inflight := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: " inflight", false: " warm"}[inflight], func(t *testing.T) {
				v, s, _, raw := oauthFixture(t)
				entered, resume := make(chan struct{}), make(chan struct{})
				done := make(chan error, 1)
				if inflight {
					s.lookup = func() { close(entered); <-resume }
					go func() { _, err := v.Verify(context.Background(), raw); done <- err }()
					select {
					case <-entered:
					case err := <-done:
						t.Fatalf("lookup never started: %v", err)
					}
				} else if _, err := v.Verify(t.Context(), raw); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "credential":
					v.Evict(tokenHash(raw))
				case "account":
					v.EvictAccount(7)
				case "grant":
					v.EvictGrant(9)
				}
				if inflight {
					close(resume)
					if err := <-done; !errors.Is(err, ErrUnauthorized) {
						t.Fatal("stale lookup accepted")
					}
					s.lookup = nil
				}
				s.oauth.GrantRevokedAt = &s.oauth.ExpiresAt
				if _, err := v.Verify(t.Context(), raw); !errors.Is(err, ErrUnauthorized) {
					t.Fatal("revoked credential cached")
				}
			})
		}
	}
}

func TestBearer_GrantEvictionPreservesOtherCredentials(t *testing.T) {
	v, s, _, raw := oauthFixture(t)
	personal := "trc_" + strings.Repeat("A", 43)
	if _, err := v.Verify(t.Context(), personal); err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	s.lookup = func() { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { _, err := v.Verify(context.Background(), raw); done <- err }()
	<-entered
	v.EvictGrant(10)
	close(resume)
	if err := <-done; err != nil {
		t.Fatal("another grant's lookup fenced")
	}
	s.lookup = nil
	s.oauthErr = errors.New("database unavailable")
	if _, err := v.Verify(t.Context(), raw); err != nil {
		t.Fatal("another grant's credential evicted")
	}
	v.EvictGrant(9)
	s.err = errors.New("database unavailable")
	if _, err := v.Verify(t.Context(), personal); err != nil {
		t.Fatal("personal credential evicted")
	}
	if len(v.grantGenerations) != 0 {
		t.Fatal("completed lookup left a grant fence")
	}
}
