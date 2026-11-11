package websession

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
)

var _ weblogin.SessionCreator = (*Manager)(nil)
var _ Store = (*store.Queries)(nil)

func TestCookie_Attributes(t *testing.T) {
	m, _, _ := harness()
	w := httptest.NewRecorder()
	if err := m.CreateSession(context.Background(), w, request(""), 42); err != nil {
		t.Fatal(err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "__Host-session" || c.Path != "/" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Domain != "" || c.MaxAge != 30*24*60*60 {
		t.Fatalf("attributes=%+v", c)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(decoded) != 32 {
		t.Fatal("invalid cookie encoding")
	}
}

func TestSession_HashedAtRest(t *testing.T) {
	m, s, _ := harness()
	m.Rand = bytes.NewReader(bytes.Repeat([]byte{9}, 64))
	w := httptest.NewRecorder()
	if err := m.CreateSession(context.Background(), w, request(""), 42); err != nil {
		t.Fatal(err)
	}
	raw := w.Result().Cookies()[0].Value
	row, ok := s.rows[hashValue(raw)]
	if !ok || row.TokenHash == raw || len(row.TokenHash) != 64 || row.SessionAuthEpoch != 3 || row.AccountID != 42 {
		t.Fatal("session was not stored under hash with account epoch")
	}
	if len(row.CsrfSecret) != 43 {
		t.Fatal("invalid CSRF secret length")
	}
}

func TestSession_SlidingExpiryAtMostHourly(t *testing.T) {
	m, s, raw := harness()
	assertStatus(t, serve(m, request(raw)), 204)
	advance(s, 2*time.Hour)
	w := serve(m, request(raw))
	assertStatus(t, w, 204)
	if s.touches != 1 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != 30*24*60*60 {
		t.Fatal("missing sliding cookie or touch")
	}
	advance(s, time.Minute)
	w = serve(m, request(raw))
	assertStatus(t, w, 204)
	if s.touches != 1 || len(w.Result().Cookies()) != 0 {
		t.Fatal("repeat request wrote or renewed cookie")
	}
	t.Run("conditional loser", func(t *testing.T) {
		m, s, raw := harness()
		advance(s, 2*time.Hour)
		s.touchZero = true
		w := serve(m, request(raw))
		assertStatus(t, w, 204)
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("renewed without writing")
		}
	})
}

func TestSession_AbsoluteNinetyDays(t *testing.T) {
	m, s, raw := harness()
	editRow(s, raw, func(r *store.GetSessionRow) {
		r.CreatedAt = s.now.Add(-89 * 24 * time.Hour)
		r.LastSeenAt = s.now.Add(-2 * time.Hour)
	})
	w := serve(m, request(raw))
	assertStatus(t, w, 204)
	if got := w.Result().Cookies()[0].MaxAge; got != 24*60*60 {
		t.Fatalf("max age=%d", got)
	}
	advance(s, 24*time.Hour)
	editRow(s, raw, func(r *store.GetSessionRow) { r.LastSeenAt = s.now; r.ExpiresAt = s.now.Add(time.Hour) })
	assertStatus(t, serve(m, request(raw)), 401)
	t.Run("creation clamps returned expiry", func(t *testing.T) {
		m, s, _ := harness()
		s.now = s.now.Add(-89 * 24 * time.Hour)
		now := s.now.Add(89 * 24 * time.Hour)
		m.Clock = func() time.Time { return now }
		w := httptest.NewRecorder()
		if err := m.CreateSession(context.Background(), w, request(""), 42); err != nil {
			t.Fatal(err)
		}
		if got := w.Result().Cookies()[0].MaxAge; got != 86400 {
			t.Fatalf("max age=%d", got)
		}
	})
}

func TestSession_RejectsRevokedExpiredBlockedEpoch(t *testing.T) {
	cases := map[string]func(*store.GetSessionRow){"revoked": func(r *store.GetSessionRow) { r.RevokedAt = &r.CreatedAt }, "expired": func(r *store.GetSessionRow) { r.ExpiresAt = r.CreatedAt }, "blocked": func(r *store.GetSessionRow) { r.BlockedAt = &r.CreatedAt }, "deleted": func(r *store.GetSessionRow) { r.DeleteRequestedAt = &r.CreatedAt }, "epoch": func(r *store.GetSessionRow) { r.AccountAuthEpoch++ }}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			m, s, raw := harness()
			editRow(s, raw, f)
			w := serve(m, request(raw))
			assertStatus(t, w, 401)
			if w.Body.String() != "{\"error\":{\"code\":\"unauthenticated\",\"message\":\"Unauthenticated\"}}\n" {
				t.Fatalf("error=%s", w.Body.String())
			}
			if s.touches != 0 {
				t.Fatal("invalid session touched")
			}
		})
	}
	for _, raw := range []string{"", "bad", rawValue(8)} {
		m, _, _ := harness()
		assertStatus(t, serve(m, request(raw)), 401)
	}
	m, _, raw := harness()
	r := request(raw)
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
	assertStatus(t, serve(m, r), 401)
}

func TestSession_CacheBoundedByWindowAndExpiry(t *testing.T) {
	for _, ttl := range []time.Duration{0, time.Hour, 3 * time.Second, -1} {
		t.Run(ttl.String(), func(t *testing.T) {
			m, s, raw := harness()
			m.CacheTTL = ttl
			assertStatus(t, serve(m, request(raw)), 204)
			editRow(s, raw, func(r *store.GetSessionRow) { r.RevokedAt = &r.CreatedAt })
			window := 10 * time.Second
			if ttl > 0 && ttl < window {
				window = ttl
			}
			if ttl >= 0 {
				advance(s, window-time.Nanosecond)
				assertStatus(t, serve(m, request(raw)), 204)
				advance(s, time.Nanosecond)
			}
			assertStatus(t, serve(m, request(raw)), 401)
		})
	}
	t.Run("expiry", func(t *testing.T) {
		m, s, raw := harness()
		editRow(s, raw, func(r *store.GetSessionRow) { r.ExpiresAt = s.now.Add(time.Second) })
		assertStatus(t, serve(m, request(raw)), 204)
		advance(s, time.Second)
		assertStatus(t, serve(m, request(raw)), 401)
	})
	t.Run("slow lookup does not extend window", func(t *testing.T) {
		m, s, raw := harness()
		s.onGet = func(context.Context) { advance(s, 9*time.Second) }
		assertStatus(t, serve(m, request(raw)), 204)
		s.onGet = nil
		editRow(s, raw, func(r *store.GetSessionRow) { r.RevokedAt = &r.CreatedAt })
		advance(s, time.Second)
		assertStatus(t, serve(m, request(raw)), 401)
	})
}

func TestSession_LocalRevokeEvictsAtOnce(t *testing.T) {
	for _, own := range []bool{false, true} {
		t.Run(map[bool]string{false: "hash", true: "own"}[own], func(t *testing.T) {
			for _, warm := range []bool{false, true} {
				t.Run(map[bool]string{false: "in flight", true: "warm"}[warm], func(t *testing.T) {
					m, s, raw := harness()
					started, release := make(chan struct{}), make(chan struct{})
					result := make(chan *httptest.ResponseRecorder, 1)
					if warm {
						assertStatus(t, serve(m, request(raw)), 204)
					} else {
						s.onGet = func(context.Context) { close(started); <-release }
						go func() { result <- serve(m, request(raw)) }()
						<-started
					}
					if own {
						count, err := m.RevokeOwnSession(context.Background(), 42, 7)
						if err != nil || count != 1 {
							t.Fatalf("revoke count=%d err=%v", count, err)
						}
					} else {
						if err := m.RevokeSession(context.Background(), raw); err != nil {
							t.Fatal(err)
						}
					}
					if !warm {
						close(release)
						assertStatus(t, <-result, 401)
						s.onGet = nil
					}
					assertStatus(t, serve(m, request(raw)), 401)
				})
			}
		})
	}
	t.Run("other account cannot revoke", func(t *testing.T) {
		m, _, raw := harness()
		assertStatus(t, serve(m, request(raw)), 204)
		count, err := m.RevokeOwnSession(context.Background(), 99, 7)
		if err != nil || count != 0 {
			t.Fatal("cross account revoke succeeded")
		}
		assertStatus(t, serve(m, request(raw)), 204)
	})
}

func TestSession_AuthDownIs503(t *testing.T) {
	for _, step := range []string{"get", "touch"} {
		t.Run(step, func(t *testing.T) {
			m, s, raw := harness()
			s.fail = step
			advance(s, 2*time.Hour)
			w := serve(m, request(raw))
			assertStatus(t, w, 503)
			if strings.Contains(w.Body.String(), "private") || !strings.Contains(w.Body.String(), `"code":"unavailable"`) {
				t.Fatal("unsafe error")
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("cookie on database error")
			}
		})
	}
	t.Run("deadline", func(t *testing.T) {
		m, s, raw := harness()
		ctx, cancel := context.WithCancel(context.Background())
		s.onGet = func(ctx context.Context) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 2*time.Second {
				t.Error("missing auth deadline")
			}
			cancel()
		}
		assertStatus(t, serve(m, request(raw).WithContext(ctx)), 503)
	})
	t.Run("nil store", func(t *testing.T) {
		m, _, raw := harness()
		m.Store = nil
		assertStatus(t, serve(m, request(raw)), 503)
	})
	t.Run("cancellation after lookup", func(t *testing.T) {
		m, s, raw := harness()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m.Clock = func() time.Time {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.reads > 0 {
				cancel()
			}
			return s.now
		}
		assertStatus(t, serve(m, request(raw).WithContext(ctx)), 503)
	})
}

func TestSession_LoginRevokesPresentedSession(t *testing.T) {
	m, s, raw := harness()
	assertStatus(t, serve(m, request(raw)), 204)
	w := httptest.NewRecorder()
	if err := m.CreateSession(context.Background(), w, request(raw), 42); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, serve(m, request(raw)), 401)
	if s.writes != 1 {
		t.Fatalf("created=%d", s.writes)
	}
	assertStatus(t, serve(m, request(w.Result().Cookies()[0].Value)), 204)
	for _, step := range []string{"revoke", "account", "create"} {
		t.Run(step, func(t *testing.T) {
			m, s, raw := harness()
			s.fail = step
			w := httptest.NewRecorder()
			err := m.CreateSession(context.Background(), w, request(raw), 42)
			if err == nil || len(w.Result().Cookies()) != 0 || strings.Contains(err.Error(), "private") {
				t.Fatal("failure issued session or leaked error")
			}
		})
	}
	t.Run("random failure", func(t *testing.T) {
		m, _, _ := harness()
		m.Rand = bytes.NewReader(nil)
		w := httptest.NewRecorder()
		if err := m.CreateSession(context.Background(), w, request(""), 42); err == nil || len(w.Result().Cookies()) != 0 {
			t.Fatal("random failure issued session")
		}
	})
}

func TestBearerNeverAuthorizesWeb(t *testing.T) {
	m, s, raw := harness()
	r := request("")
	r.Header.Set("Authorization", "Bearer "+raw)
	assertStatus(t, serve(m, r), 401)
	if s.reads != 0 {
		t.Fatal("bearer queried session")
	}
}
