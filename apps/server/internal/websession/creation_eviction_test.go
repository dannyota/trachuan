package websession

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestCreateSession_PreservesOtherLiveSessionCache(t *testing.T) {
	m, s, raw := harness()
	assertStatus(t, serve(m, request(raw)), 204)
	if err := m.CreateSession(context.Background(), httptest.NewRecorder(), request(""), 42); err != nil {
		t.Fatal(err)
	}
	s.fail = "get"
	assertStatus(t, serve(m, request(raw)), 204)
}

type retiringSessionStore struct{ *fakeStore }

func (s retiringSessionStore) CreateSession(ctx context.Context, p store.CreateSessionParams) (store.CreateSessionRow, error) {
	row, err := s.fakeStore.CreateSession(ctx, p)
	if err != nil {
		return row, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	digest := hashValue(rawValue(1))
	old := s.rows[digest]
	old.RevokedAt = &s.now
	s.rows[digest] = old
	row.RevokedHashes = []string{digest}
	return row, nil
}

func TestCreateSession_EvictsOldestWarmAndInFlight(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(fmt.Sprint(warm), func(t *testing.T) {
			m, s, raw := harness()
			m.Store = retiringSessionStore{s}
			other := rawValue(3)
			row := s.rows[hashValue(raw)]
			row.ID, row.TokenHash = 8, hashValue(other)
			s.rows[hashValue(other)] = row
			assertStatus(t, serve(m, request(other)), 204)
			entered, resume := make(chan struct{}), make(chan struct{})
			var result chan *httptest.ResponseRecorder
			if warm {
				assertStatus(t, serve(m, request(raw)), 204)
			} else {
				s.onGet = func(context.Context) { close(entered); <-resume }
				result = make(chan *httptest.ResponseRecorder, 1)
				go func() { result <- serve(m, request(raw)) }()
				<-entered
			}
			if err := m.CreateSession(context.Background(), httptest.NewRecorder(), request(""), 42); err != nil {
				t.Fatal(err)
			}
			if !warm {
				close(resume)
				assertStatus(t, <-result, 401)
				s.onGet = nil
			}
			assertStatus(t, serve(m, request(raw)), 401)
			s.fail = "get"
			assertStatus(t, serve(m, request(other)), 204)
		})
	}
}
