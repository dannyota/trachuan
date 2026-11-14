package passwordauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"time"
)

func TestFactor_BodyAndCSRFFailures(t *testing.T) {
	for _, session := range []bool{false, true} {
		for _, scenario := range []string{"missing csrf", "wrong csrf", "duplicate csrf", "missing origin", "wrong origin", "duplicate origin", "both codes", "neither code", "null code", "number code", "null body", "trailing JSON", "unknown field", "wrong content type"} {
			t.Run(scenario+map[bool]string{true: " session", false: " pending"}[session], func(t *testing.T) {
				sessions := &websession.Manager{Store: sessionStore{}, PublicURL: "https://example.test"}
				h := Handler{Factors: factorFlow{}, Sessions: sessions, PublicURL: sessions.PublicURL}
				body := `{"code":"123456"}`
				switch scenario {
				case "both codes":
					body = `{"code":"123456","recovery_code":"synthetic"}`
				case "neither code":
					body = `{}`
				case "null code":
					body = `{"code":null}`
				case "number code":
					body = `{"code":123456}`
				case "null body":
					body = `null`
				case "trailing JSON":
					body += ` {}`
				case "unknown field":
					body = `{"code":"123456","extra":true}`
				}
				path, method := "/api/v1/auth/factor", "POST"
				if session {
					path, method = "/api/v1/me/totp", "DELETE"
				}
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("A", 43)})
				r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
				r.Header.Set("Origin", sessions.PublicURL)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				want := 400
				switch scenario {
				case "missing csrf":
					r.Header.Del("X-CSRF-Token")
					want = 403
				case "wrong csrf":
					r.Header.Set("X-CSRF-Token", strings.Repeat("B", 43))
					want = 403
				case "duplicate csrf":
					r.Header.Add("X-CSRF-Token", strings.Repeat("A", 43))
					want = 403
				case "missing origin":
					r.Header.Del("Origin")
					want = 403
				case "wrong origin":
					r.Header.Set("Origin", "https://hostile.test")
					want = 403
				case "duplicate origin":
					r.Header.Add("Origin", sessions.PublicURL)
					want = 403
				case "wrong content type":
					r.Header.Set("Content-Type", "text/plain")
					want = 415
					if session {
						want = 403
					}
				}
				var handler http.Handler = http.HandlerFunc(h.Factor)
				if session {
					handler = sessions.Middleware(sessions.CSRF(http.HandlerFunc(h.TOTPRemove)))
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != want {
					t.Fatalf("got %d want %d", w.Code, want)
				}
			})
		}
	}
}

type capturedRecovery struct {
	factorFlow
	present bool
	raw     string
}

func (s *capturedRecovery) CompleteFactor(_ context.Context, p accountflow.FactorCompletion) (store.CreateSessionRow, string, error) {
	s.present, s.raw = p.UseRecovery, p.RecoveryCode
	return contractFlow{}.result(), "/", nil
}
func TestFactor_EmptyRecoveryRemainsRecoveryAttempt(t *testing.T) {
	s := &capturedRecovery{}
	h := Handler{Factors: s, Sessions: &websession.Manager{}, PublicURL: "https://example.test"}
	r := httptest.NewRequest("POST", "/api/v1/auth/factor", strings.NewReader(`{"recovery_code":""}`))
	r.Header.Set("Origin", h.PublicURL)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("A", 43)})
	w := httptest.NewRecorder()
	h.Factor(w, r)
	if w.Code != 200 || !s.present || s.raw != "" {
		t.Fatal("empty recovery input lost its attempt kind")
	}
}
func TestLogin_EnrolledAnswerMatchesContract(t *testing.T) {
	hash, version := "synthetic", int64(1)
	h := Handler{Store: fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}, Hasher: &fakeHasher{match: true}, Counter: password.NewCounter(time.Now), Sessions: &fakeSessions{err: &store.PendingRequired{CSRF: strings.Repeat("A", 43)}}, PublicURL: "https://example.test"}
	r := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"email":"person@example.test","password":"synthetic passphrase"}`))
	r.Header.Set("Origin", h.PublicURL)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r)
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	contracttest.NewCoverageForTag(c, "password").Check(t, "POST", "/api/v1/auth/password", w)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"factor_required":true`) || strings.Contains(w.Body.String(), `"next"`) {
		t.Fatal("pending answer does not match contract")
	}
}

type expiredCompletion struct {
	factorFlow
	expired  bool
	keepLive bool
}

func (s *expiredCompletion) PendingFactor(ctx context.Context, hash string) (store.PendingAuth, error) {
	if s.expired {
		return store.PendingAuth{}, factor.ErrFailed
	}
	return s.factorFlow.PendingFactor(ctx, hash)
}
func (s *expiredCompletion) CompleteFactor(context.Context, accountflow.FactorCompletion) (store.CreateSessionRow, string, error) {
	s.expired = !s.keepLive
	return store.CreateSessionRow{}, "", factor.ErrFailed
}
func TestFactor_ExpiryDuringCompletionClearsCookie(t *testing.T) {
	for _, keepLive := range []bool{false, true} {
		s := &expiredCompletion{keepLive: keepLive}
		h := Handler{Factors: s, Sessions: &websession.Manager{}, PublicURL: "https://example.test"}
		r := httptest.NewRequest("POST", "/api/v1/auth/factor", strings.NewReader(`{"code":"123456"}`))
		r.Header.Set("Origin", h.PublicURL)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
		r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("A", 43)})
		w := httptest.NewRecorder()
		h.Factor(w, r)
		if w.Code != 401 {
			t.Fatal("expired completion status")
		}
		cleared := false
		for _, cookie := range w.Result().Cookies() {
			if cookie.Name == "__Host-pending" && cookie.MaxAge < 0 {
				cleared = true
			}
		}
		if cleared == keepLive {
			t.Fatalf("keep live=%v, cleared cookie=%v", keepLive, cleared)
		}
	}
}
