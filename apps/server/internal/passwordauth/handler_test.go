package passwordauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type fakeStore struct {
	row store.PasswordSnapshot
	err error
}

func (s fakeStore) GetPasswordByEmail(context.Context, string) (store.PasswordSnapshot, error) {
	return s.row, s.err
}

type fakeHasher struct {
	calls int
	match bool
	err   error
}

func (h *fakeHasher) Verify(context.Context, string, string) (bool, error) {
	h.calls++
	return h.match, h.err
}
func (h *fakeHasher) VerifyDummy(context.Context, string) error { h.calls++; return h.err }

type fakeSessions struct {
	err   error
	calls int
}

func (s *fakeSessions) CreatePasswordSession(context.Context, http.ResponseWriter, *http.Request, store.PasswordSnapshot) error {
	s.calls++
	return s.err
}

func TestLogin_UniformFailureAndQueue(t *testing.T) {
	hash := "synthetic PHC"
	version := int64(1)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	var expected string
	for _, scenario := range []string{"unknown", "passwordless", "wrong", "blocked", "counter", "busy known", "busy unknown"} {
		t.Run(scenario, func(t *testing.T) {
			db := fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}
			h := &fakeHasher{match: true}
			sessions := &fakeSessions{}
			counter := password.NewCounter(func() time.Time { return now })
			switch scenario {
			case "unknown", "busy unknown":
				db.err = pgx.ErrNoRows
			case "passwordless":
				db.row.Hash = nil
			case "wrong":
				h.match = false
			case "blocked":
				sessions.err = store.ErrLoginFailed
			case "counter":
				for range 10 {
					counter.Allow("person@example.test")
				}
			}
			if strings.HasPrefix(scenario, "busy") {
				h.err = password.ErrBusy
			}
			handler := Handler{Store: db, Hasher: h, Counter: counter, Sessions: sessions, PublicURL: "https://example.test"}
			request := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"email":"person@EXAMPLE.TEST","password":"synthetic passphrase"}`))
			request.Header.Set("Origin", "https://example.test")
			request.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.Login(w, request)
			if strings.HasPrefix(scenario, "busy") {
				if w.Code != 503 || !strings.Contains(w.Body.String(), "busy") {
					t.Fatal("queue response", w.Code)
				}
				return
			}
			if w.Code != 401 || len(w.Result().Cookies()) != 0 {
				t.Fatal("failure issued session", w.Code)
			}
			if expected == "" {
				expected = w.Body.String()
			}
			if expected != w.Body.String() {
				t.Fatal("failure differs")
			}
			if h.calls != 1 {
				t.Fatal("missing hash work")
			}
		})
	}
}
func TestLogin_PreSessionGuardsAndNext(t *testing.T) {
	hash := "synthetic"
	version := int64(1)
	for _, tc := range []struct {
		origin, media, body string
		status              int
	}{
		{"", "application/json", `{}`, 403}, {"https://other.example.test", "application/json", `{}`, 403},
		{"https://example.test", "text/plain", `{}`, 415}, {"https://example.test", "application/json", `{`, 400},
		{"https://example.test", "application/json", `{"email":"person@example.test","password":"synthetic passphrase","next":"/oauth/consent/synthetic"}`, 200},
	} {
		handler := Handler{Store: fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}, Hasher: &fakeHasher{match: true}, Counter: password.NewCounter(time.Now), Sessions: &fakeSessions{}, PublicURL: "https://example.test"}
		r := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(tc.body))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.media)
		w := httptest.NewRecorder()
		handler.Login(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d", w.Code, tc.status)
		}
	}

}
func TestLogin_DatabaseFailure(t *testing.T) {
	h := Handler{Store: fakeStore{err: errors.New("private")}, Hasher: &fakeHasher{}, Counter: password.NewCounter(time.Now), PublicURL: "https://example.test"}
	r := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"email":"person@example.test","password":"synthetic passphrase"}`))
	r.Header.Set("Origin", "https://example.test")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("unsafe database failure")
	}
}

func TestLogin_RequiresFieldsAndBoundsTrailingBody(t *testing.T) {
	h := Handler{Store: fakeStore{err: pgx.ErrNoRows}, Hasher: &fakeHasher{}, Counter: password.NewCounter(time.Now), PublicURL: "https://example.test"}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{}`, 400}, {`{"email":"person@example.test"}`, 400}, {`{"email":"person@example.test","password":null}`, 400},
		{`{"email":"person@example.test","password":"synthetic passphrase"}` + strings.Repeat(" ", 65536), 413},
	} {
		r := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(tc.body))
		r.Header.Set("Origin", h.PublicURL)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Login(w, r)
		if w.Code != tc.status {
			t.Errorf("got %d want %d", w.Code, tc.status)
		}
	}
}

func TestLogin_NextMatchesGoogle(t *testing.T) {
	hash := "synthetic"
	version := int64(1)
	for _, tc := range []struct{ raw, want string }{
		{"/account", "/account"}, {"/destination?view=recent#section", "/destination?view=recent#section"},
		{"/oauth/consent/synthetic?lang=en", "/oauth/consent/synthetic?lang=en"},
		{"", "/"}, {"//other.example.test", "/"}, {"/\\other.example.test", "/"},
		{"/oauth/consent/../bad", "/"}, {"/oauth/consent/%2e%2e", "/"},
		{"/invalid%escape", "/"}, {"/%00", "/"}, {strings.Repeat("/", 513), "/"},
		{"https://example.test/account", "/"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			handler := Handler{Store: fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}, Hasher: &fakeHasher{match: true}, Counter: password.NewCounter(time.Now), Sessions: &fakeSessions{}, PublicURL: "https://example.test"}
			body, err := json.Marshal(map[string]string{"email": "person@example.test", "password": "synthetic passphrase", "next": tc.raw})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(string(body)))
			r.Header.Set("Origin", handler.PublicURL)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.Login(w, r)
			var response struct {
				Next string `json:"next"`
			}
			if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || response.Next != tc.want {
				t.Errorf("status=%d next=%q want=%q", w.Code, response.Next, tc.want)
			}
		})
	}
}
