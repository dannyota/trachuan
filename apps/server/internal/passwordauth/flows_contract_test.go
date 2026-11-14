package passwordauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type contractFlow struct{ *mailFlow }

func (s contractFlow) CompleteRegistration(context.Context, accountflow.Completion) (store.CreateSessionRow, error) {
	return s.result(), s.err
}
func (s contractFlow) CompleteReset(context.Context, accountflow.Completion) (int64, error) {
	return 1, s.err
}
func (s contractFlow) RequestAddPassword(context.Context, accountflow.Acting) error { return s.err }
func (s contractFlow) ChangePassword(context.Context, accountflow.Completion) (store.CreateSessionRow, error) {
	return s.result(), s.err
}
func (contractFlow) result() store.CreateSessionRow {
	return store.CreateSessionRow{AccountID: 1, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
}
func TestContract_AccountPasswordHandlers(t *testing.T) {
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(contract, "account-password")
	for _, tc := range []struct {
		path, body string
		signed     bool
		statuses   []int
	}{
		{"/api/v1/auth/signup", `{"email":"person@example.test"}`, false, []int{202, 400, 403, 413, 415, 503}},
		{"/api/v1/auth/reset", `{"email":"person@example.test"}`, false, []int{202, 400, 403, 413, 415, 503}},
		{"/api/v1/auth/link", `{"token":"synthetic"}`, false, []int{200, 400, 403, 409, 413, 415, 503}},
		{"/api/v1/auth/signup/complete", `{"token":"synthetic","password":"synthetic passphrase alpha"}`, false, []int{200, 400, 403, 409, 413, 415, 503}},
		{"/api/v1/auth/reset/complete", `{"token":"synthetic","password":"synthetic passphrase alpha"}`, false, []int{204, 400, 403, 409, 413, 415, 503}},
		{"/api/v1/me/password/link", "", true, []int{202, 400, 401, 403, 503}},
		{"/api/v1/me/password", `{"current":"synthetic passphrase alpha","password":"synthetic passphrase beta"}`, true, []int{200, 400, 401, 403, 409, 413, 429, 503}},
	} {
		for _, status := range tc.statuses {
			flow := contractFlow{&mailFlow{link: accountflow.Link{Purpose: "registration", Email: "person@example.test"}}}
			hash, version := "old", int64(1)
			sessions := &websession.Manager{Store: sessionStore{}, PublicURL: "https://example.test"}
			h := Handler{Flows: flow, Store: fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}, Hasher: &fakeHasher{match: true}, Counter: password.NewCounter(time.Now), Proofs: &proofStore{}, Sessions: sessions, PublicURL: sessions.PublicURL, HashPassword: func(context.Context, string) (string, error) { return "new", nil }}
			method := map[string]http.HandlerFunc{
				"/api/v1/auth/signup": h.Signup, "/api/v1/auth/reset": h.Reset, "/api/v1/auth/link": h.Link,
				"/api/v1/auth/signup/complete": h.SignupComplete, "/api/v1/auth/reset/complete": h.ResetComplete,
				"/api/v1/me/password/link": h.PasswordLink, "/api/v1/me/password": h.Password,
			}[tc.path]
			body := tc.body
			switch status {
			case 400:
				if tc.path == "/api/v1/me/password/link" {
					flow.err = accountflow.ErrInvalidRequest
				} else {
					body = "{"
				}
			case 409:
				flow.err = accountflow.ErrLinkInvalid
			case 413:
				body = `{"password":"` + strings.Repeat("x", 65536) + `"}`
			case 429:
				flow.err = accountflow.ErrRateLimited
			case 503:
				flow.err = errors.New("synthetic unavailable")
			}
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(body))
			req.Header.Set("Origin", h.PublicURL)
			req.Header.Set("Content-Type", "application/json")
			var handler http.Handler = method
			if tc.signed {
				req.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
				req.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				if status == 401 {
					req.Header.Del("Cookie")
				}
				handler = sessions.Middleware(sessions.CSRF(handler))
			}
			if status == 403 {
				req.Header.Del("Origin")
			}
			if status == 415 {
				req.Header.Set("Content-Type", "text/plain")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != status {
				t.Fatalf("%s status=%d want=%d", tc.path, w.Code, status)
			}
			coverage.Check(t, "POST", tc.path, w)
		}
	}
	missing := coverage.Missing()
	if len(missing) != 5 {
		t.Fatalf("handler coverage missing: %v", missing)
	}
	for _, item := range missing {
		if !strings.Contains(item, "429") {
			t.Fatalf("handler response untested: %s", item)
		}
	}
}
