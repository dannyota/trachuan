package passwordauth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestContract_PasswordHandlers(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(c, "password")
	hash := "synthetic"
	version := int64(1)
	for _, reauth := range []bool{false, true} {
		statuses := []int{200, 400, 401, 403, 413, 415, 503}
		route := "/api/v1/auth/password"
		if reauth {
			route = "/api/v1/me/reauth"
			statuses = []int{204, 400, 401, 403, 413, 503}
		}
		for _, status := range statuses {
			hasher := &fakeHasher{match: true}
			h := Handler{Store: fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}, Hasher: hasher, Counter: password.NewCounter(time.Now), Sessions: &fakeSessions{}, Proofs: &proofStore{}, PublicURL: "https://example.test"}
			body := `{"email":"person@example.test","password":"synthetic passphrase"}`
			if reauth {
				body = `{"password":"synthetic passphrase"}`
			}
			if status == 400 {
				body = "{"
			}
			if status == 413 {
				body = `{"password":"` + strings.Repeat("a", 65536) + `"}`
			}
			if status == 401 {
				hasher.match = false
			}
			if status == 503 {
				hasher.err = password.ErrBusy
			}
			r := httptest.NewRequest("POST", route, strings.NewReader(body))
			r.Header.Set("Origin", h.PublicURL)
			r.Header.Set("Content-Type", "application/json")
			if reauth {
				r = reauthRequest(body)
			}
			if status == 403 {
				r.Header.Del("Origin")
				r.Header.Del("X-CSRF-Token")
			}
			if status == 415 {
				r.Header.Set("Content-Type", "text/plain")
			}
			w := httptest.NewRecorder()
			if reauth {
				reauthFixture(&h).ServeHTTP(w, r)
			} else {
				h.Login(w, r)
			}
			if w.Code != status {
				t.Fatalf("%s got %d want %d", route, w.Code, status)
			}
			coverage.Check(t, "POST", route, w)
		}
	}
	missing := coverage.Missing()
	if len(missing) != 1 || !strings.Contains(missing[0], "429") {
		t.Fatalf("missing handler coverage: %v", missing)
	}
}
