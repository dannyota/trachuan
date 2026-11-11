package websession

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSession_RevokeLeavesOtherSessionsAvailable(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "in flight", true: "cached"}[warm], func(t *testing.T) {
			m, s, raw := harness()
			other := rawValue(8)
			row := s.rows[hashValue(raw)]
			row.ID = 8
			row.TokenHash = hashValue(other)
			s.rows[row.TokenHash] = row
			if warm {
				assertStatus(t, serve(m, request(other)), 204)
			}
			started, release := make(chan struct{}), make(chan struct{})
			lookup := make(chan *httptest.ResponseRecorder, 1)
			if !warm {
				s.onGet = func(context.Context) { close(started); <-release }
				go func() { lookup <- serve(m, request(other)) }()
				<-started
			}
			revoked := make(chan error, 1)
			revoking, finish := make(chan struct{}), make(chan struct{})
			s.onRevoke = func(context.Context) { close(revoking); <-finish }
			go func() { revoked <- m.RevokeSession(context.Background(), raw) }()
			<-revoking
			if warm {
				assertStatus(t, serve(m, request(other)), 204)
			}
			close(finish)
			if err := <-revoked; err != nil {
				t.Fatal(err)
			}
			if !warm {
				close(release)
				assertStatus(t, <-lookup, 204)
			}
		})
	}
}

func TestFakeStore_TouchOnlyLiveSessions(t *testing.T) {
	for _, state := range []string{"revoked", "expired", "missing"} {
		t.Run(state, func(t *testing.T) {
			_, s, raw := harness()
			advance(s, 2*time.Hour)
			s.mu.Lock()
			row := s.rows[hashValue(raw)]
			switch state {
			case "revoked":
				row.RevokedAt = &s.now
				s.rows[hashValue(raw)] = row
			case "expired":
				row.ExpiresAt = s.now
				s.rows[hashValue(raw)] = row
			case "missing":
				delete(s.rows, hashValue(raw))
			}
			s.mu.Unlock()
			count, err := s.TouchSession(context.Background(), hashValue(raw))
			if err != nil || count != 0 {
				t.Fatalf("count=%d err=%v", count, err)
			}
		})
	}
}

func TestSession_RevokeDuringTouchOrReread(t *testing.T) {
	for _, step := range []string{"touch", "reread"} {
		for _, own := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/own=%t", step, own), func(t *testing.T) {
				m, s, raw := harness()
				advance(s, 2*time.Hour)
				started, release := make(chan struct{}), make(chan struct{})
				if step == "touch" {
					s.onTouch = func(context.Context) { close(started); <-release }
				} else {
					s.onGet = func(context.Context) {
						s.mu.Lock()
						reads := s.reads
						s.mu.Unlock()
						if reads == 2 {
							close(started)
							<-release
						}
					}
				}
				result := make(chan *httptest.ResponseRecorder, 1)
				go func() { result <- serve(m, request(raw)) }()
				<-started
				if own {
					count, err := m.RevokeOwnSession(context.Background(), 42, 7)
					if err != nil || count != 1 {
						t.Fatalf("revoke count=%d err=%v", count, err)
					}
				} else if err := m.RevokeSession(context.Background(), raw); err != nil {
					t.Fatal(err)
				}
				close(release)
				w := <-result
				assertStatus(t, w, 401)
				if len(w.Result().Cookies()) != 0 {
					t.Fatal("revoked session renewed")
				}
				s.mu.Lock()
				touches := s.touches
				s.mu.Unlock()
				if step == "touch" && touches != 0 {
					t.Fatalf("revoked session touched %d times", touches)
				}
				m.mu.Lock()
				_, cached := m.entries[hashValue(raw)]
				m.mu.Unlock()
				if cached {
					t.Fatal("revoked session cached")
				}
				s.mu.Lock()
				s.onTouch = nil
				s.onGet = nil
				s.mu.Unlock()
				assertStatus(t, serve(m, request(raw)), 401)
			})
		}
	}
}
