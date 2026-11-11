package websession

import (
	"context"
	"net/http/httptest"
	"testing"
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
