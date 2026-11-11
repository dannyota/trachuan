package websession

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func evictAccount(t *testing.T, m *Manager, id int64) {
	t.Helper()
	e, ok := any(m).(interface{ EvictAccount(int64) })
	if !ok {
		t.Fatal("missing account eviction")
	}
	e.EvictAccount(id)
}

func TestEvictAccount_WarmAndOtherAccount(t *testing.T) {
	m, s, raw := harness()
	other := rawValue(3)
	row := s.rows[hashValue(raw)]
	row.ID, row.AccountID, row.TokenHash = 8, 43, hashValue(other)
	s.rows[hashValue(other)] = row
	for _, token := range []string{raw, other} {
		if _, _, err := m.lookup(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	evictAccount(t, m, 42)
	s.fail = "get"
	if _, _, err := m.lookup(context.Background(), raw); err == nil {
		t.Fatal("account session cached")
	}
	if _, _, err := m.lookup(context.Background(), other); err != nil {
		t.Fatal("other account evicted")
	}
}

func TestEvictAccount_InFlight(t *testing.T) {
	m, s, raw := harness()
	entered, resume := make(chan struct{}), make(chan struct{})
	s.onGet = func(context.Context) { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { _, _, err := m.lookup(context.Background(), raw); done <- err }()
	<-entered
	evictAccount(t, m, 42)
	close(resume)
	if err := <-done; err != errUnauthenticated {
		t.Fatal("stale lookup accepted")
	}
	s.onGet = nil
	s.rows = map[string]store.GetSessionRow{}
	if _, _, err := m.lookup(context.Background(), raw); err != errUnauthenticated {
		t.Fatal("stale lookup cached")
	}
}

func TestEvictAccount_OtherAccountInFlight(t *testing.T) {
	m, s, raw := harness()
	entered, resume := make(chan struct{}), make(chan struct{})
	s.onGet = func(context.Context) { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { _, _, err := m.lookup(context.Background(), raw); done <- err }()
	<-entered
	m.EvictAccount(43)
	close(resume)
	if err := <-done; err != nil {
		t.Fatal("other account lookup rejected")
	}
	s.onGet = nil
	s.fail = "get"
	if _, _, err := m.lookup(context.Background(), raw); err != nil {
		t.Fatal("other account lookup not cached")
	}
}
