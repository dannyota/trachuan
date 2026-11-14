package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMailLimiter_RollingHourAndIndependentRoutes(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	source := newLoginLimiter(func() time.Time { return now }, nil)
	signup, reset := newMailLimiter(source), newMailLimiter(source)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(202) })
	call := func(l *mailLimiter) int {
		w := httptest.NewRecorder()
		l.middleware(next).ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
		return w.Code
	}
	for range 5 {
		if call(signup) != 202 {
			t.Fatal("early refusal")
		}
	}
	now = now.Add(59 * time.Minute)
	if call(signup) != 429 || call(reset) != 202 {
		t.Fatal("window or route separation failed")
	}
	now = now.Add(time.Minute)
	if call(signup) != 202 {
		t.Fatal("window failed to expire")
	}
}
