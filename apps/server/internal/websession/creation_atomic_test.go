package websession

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestCreateSession_FailurePreservesPresentedSession(t *testing.T) {
	m, s, raw := harness()
	assertStatus(t, serve(m, request(raw)), 204)
	s.fail = "create"
	w := httptest.NewRecorder()
	if err := m.CreateSession(context.Background(), w, request(raw), 42); err == nil {
		t.Fatal("creation failure ignored")
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("failed creation set cookie")
	}
	s.fail = ""
	assertStatus(t, serve(m, request(raw)), 204)
}
