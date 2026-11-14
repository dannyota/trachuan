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
	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type factorFlow struct{ err error }

func (s factorFlow) PendingFactor(context.Context, string) (store.PendingAuth, error) {
	next := "/"
	return store.PendingAuth{CsrfSecret: strings.Repeat("A", 43), Next: &next}, s.err
}
func (s factorFlow) CompleteFactor(context.Context, accountflow.FactorCompletion) (store.CreateSessionRow, string, error) {
	return contractFlow{}.result(), "/", s.err
}
func (s factorFlow) SetupTOTP(context.Context, accountflow.Acting) (accountflow.TOTPSetup, error) {
	return accountflow.TOTPSetup{URI: "otpauth://totp/test", Secret: "synthetic"}, s.err
}
func (s factorFlow) ConfirmTOTP(context.Context, accountflow.FactorChange) (store.CreateSessionRow, []string, error) {
	return contractFlow{}.result(), []string{"synthetic"}, s.err
}
func (s factorFlow) RemoveTOTP(context.Context, accountflow.FactorChange) (store.CreateSessionRow, error) {
	return contractFlow{}.result(), s.err
}
func (s factorFlow) ReplaceRecoveryCodes(context.Context, accountflow.FactorChange) (store.CreateSessionRow, []string, error) {
	return contractFlow{}.result(), []string{"synthetic"}, s.err
}

func TestFactor_RouteStatusMatrix(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(c, "factor")
	for _, route := range []struct {
		method, path, body string
		session            bool
		statuses           []int
	}{
		{"GET", "/api/v1/auth/factor", "", false, []int{200, 401, 503}},
		{"POST", "/api/v1/auth/factor", `{"code":"123456"}`, false, []int{200, 400, 401, 403, 413, 415, 429, 503}},
		{"POST", "/api/v1/me/totp/setup", `{}`, true, []int{200, 401, 403, 404, 503}},
		{"POST", "/api/v1/me/totp/confirm", `{"code":"123456"}`, true, []int{200, 400, 401, 403, 404, 413, 429, 503}},
		{"DELETE", "/api/v1/me/totp", `{"recovery_code":"synthetic"}`, true, []int{200, 400, 401, 403, 413, 429, 503}},
		{"POST", "/api/v1/me/recovery-codes", `{"code":"123456"}`, true, []int{200, 400, 401, 403, 413, 429, 503}},
	} {
		for _, status := range route.statuses {
			t.Run(route.path+http.StatusText(status), func(t *testing.T) {
				db := factorFlow{}
				sessions := &websession.Manager{Store: sessionStore{}, PublicURL: "https://example.test"}
				h := &Handler{Factors: db, Enrollment: true, Sessions: sessions, PublicURL: sessions.PublicURL}
				body := route.body
				switch status {
				case 400:
					body = `{"extra":true}`
				case 401:
					db.err = factor.ErrFailed
				case 413:
					body = `{"code":"` + strings.Repeat("x", 65536) + `"}`
				case 429:
					db.err = &factor.Limited{Wait: time.Minute}
				case 503:
					db.err = errors.New("synthetic unavailable")
				case 404:
					h.Enrollment = false
				}
				h.Factors = db
				r := httptest.NewRequest(route.method, route.path, strings.NewReader(body))
				r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("A", 43)})
				r.Header.Set("Origin", sessions.PublicURL)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				var method http.HandlerFunc
				switch route.path {
				case "/api/v1/auth/factor":
					if route.method == "GET" {
						method = h.FactorGet
					} else {
						method = h.Factor
					}
				case "/api/v1/me/totp/setup":
					method = h.TOTPSetup
				case "/api/v1/me/totp/confirm":
					method = h.TOTPConfirm
				case "/api/v1/me/totp":
					method = h.TOTPRemove
				default:
					method = h.RecoveryCodes
				}
				var handler http.Handler = method
				if route.session {
					r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
					handler = sessions.Middleware(sessions.CSRF(method))
					if status == 401 && route.path == "/api/v1/me/totp/setup" {
						r.Header.Del("Cookie")
					}
				}
				if status == 403 {
					r.Header.Set("Origin", "https://hostile.test")
				}
				if status == 415 {
					r.Header.Set("Content-Type", "text/plain")
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != status {
					t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
				}
				coverage.Check(t, route.method, route.path, w)
				if status >= 400 {
					for _, cookie := range w.Result().Cookies() {
						if cookie.Name == "__Host-session" {
							t.Fatal("failure issued session")
						}
					}
				}
			})
		}
	}
	if missing := coverage.Missing(); len(missing) > 0 {
		t.Fatalf("untested responses: %v", missing)
	}
}
