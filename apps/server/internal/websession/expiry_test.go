package websession

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestSession_ExpiryDuringCacheLookup(t *testing.T) {
	for _, bound := range []string{"expiry", "cache window"} {
		t.Run(bound, func(t *testing.T) {
			m, s, raw := harness()
			window := 10 * time.Second
			if bound == "expiry" {
				window = time.Second
				editRow(s, raw, func(row *store.GetSessionRow) { row.ExpiresAt = s.now.Add(window) })
			}
			assertStatus(t, serve(m, request(raw)), 204)
			if bound == "cache window" {
				editRow(s, raw, func(row *store.GetSessionRow) { row.RevokedAt = &row.CreatedAt })
			}
			sampled := make(chan struct{})
			var once sync.Once
			m.Clock = func() time.Time {
				s.mu.Lock()
				now := s.now
				s.mu.Unlock()
				once.Do(func() { close(sampled) })
				return now
			}
			m.mu.Lock()
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- serve(m, request(raw)) }()
			<-sampled
			advance(s, window)
			m.mu.Unlock()
			assertStatus(t, <-result, 401)
		})
	}
}
