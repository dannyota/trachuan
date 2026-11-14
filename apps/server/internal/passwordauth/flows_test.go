package passwordauth

import (
	"context"
	"encoding/json"
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

type mailFlow struct {
	FlowStore
	email, purpose string
	link           accountflow.Link
	err            error
}

func (s *mailFlow) RequestMail(_ context.Context, purpose, email string) error {
	s.purpose, s.email = purpose, email
	return s.err
}
func (s *mailFlow) LookupLink(context.Context, string) (accountflow.Link, error) {
	return s.link, s.err
}
func (s *mailFlow) ChangePassword(context.Context, accountflow.Completion) (store.CreateSessionRow, error) {
	return store.CreateSessionRow{}, s.err
}

func TestMailRequests_UniformAndCanonical(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(c, "account-password")
	for _, reset := range []bool{false, true} {
		var first string
		for _, email := range []string{"person@EXAMPLE.TEST", "unknown@example.test", "invalid"} {
			flow := &mailFlow{}
			h := Handler{Flows: flow, PublicURL: "https://example.test"}
			path := "/api/v1/auth/signup"
			method := h.Signup
			purpose := "registration"
			if reset {
				path = "/api/v1/auth/reset"
				method = h.Reset
				purpose = "reset"
			}
			req := httptest.NewRequest("POST", path, strings.NewReader(`{"email":"`+email+`"}`))
			req.Header.Set("Origin", h.PublicURL)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			method(w, req)
			if first == "" {
				first = w.Body.String()
			}
			if w.Code != 202 || w.Body.String() != first || len(w.Result().Cookies()) != 0 {
				t.Fatal("mail answer differs")
			}
			if email == "person@EXAMPLE.TEST" && flow.email != "person@example.test" || email == "invalid" && flow.email != "" || flow.purpose != purpose {
				t.Fatal("parser or route mismatch")
			}
			coverage.Check(t, "POST", path, w)
		}
	}
}
func TestLinkLookup_ReadOnlyAndErrors(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(c, "account-password")
	for _, tc := range []struct {
		body, origin, media string
		status              int
		err                 error
	}{
		{`{"token":"synthetic"}`, "https://example.test", "application/json", 200, nil},
		{`{"token":"synthetic"}`, "https://example.test", "application/json", 409, accountflow.ErrLinkInvalid},
		{`{}`, "https://example.test", "application/json", 400, nil},
		{`{"token":5}`, "https://example.test", "application/json", 400, nil},
		{`{"token":"synthetic"}`, "", "application/json", 403, nil},
		{`{"token":"synthetic"}`, "https://example.test", "text/plain", 415, nil},
		{`{"token":"` + strings.Repeat("x", 65536) + `"}`, "https://example.test", "application/json", 413, nil},
	} {
		h := Handler{Flows: &mailFlow{link: accountflow.Link{Purpose: "registration", Email: "person@example.test"}, err: tc.err}, PublicURL: "https://example.test"}
		req := httptest.NewRequest("POST", "/api/v1/auth/link", strings.NewReader(tc.body))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", tc.media)
		w := httptest.NewRecorder()
		h.Link(w, req)
		if w.Code != tc.status || len(w.Result().Cookies()) != 0 {
			t.Fatalf("link status=%d want=%d", w.Code, tc.status)
		}
		coverage.Check(t, "POST", "/api/v1/auth/link", w)
	}
}
func TestCompletion_RejectsPasswordBeforeLinkLookup(t *testing.T) {
	for _, reset := range []bool{false, true} {
		h := Handler{PublicURL: "https://example.test"}
		method := h.SignupComplete
		if reset {
			method = h.ResetComplete
		}
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"token":"unknown","password":"short"}`))
		r.Header.Set("Origin", h.PublicURL)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		method(w, r)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "password_rejected") {
			t.Fatal("policy did not precede link access")
		}
	}
}
func TestPasswordChange_SharedCounterAndFactorRefusal(t *testing.T) {
	hash, version := "synthetic", int64(1)
	for _, capped := range []bool{false, true} {
		counter := password.NewCounter(time.Now)
		if capped {
			for range 10 {
				finish, _ := counter.Reserve("person@example.test")
				finish(false)
			}
		}
		sessions := &websession.Manager{Store: sessionStore{}, PublicURL: "https://example.test"}
		h := Handler{Flows: &mailFlow{err: store.ErrFactorRequired}, Store: fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}, Proofs: &proofStore{}, Hasher: &fakeHasher{match: true}, Counter: counter, Sessions: sessions, HashPassword: func(context.Context, string) (string, error) { return "new", nil }}
		handler := sessions.Middleware(sessions.CSRF(http.HandlerFunc(h.Password)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, reauthRequest(`{"current":"synthetic passphrase alpha","password":"synthetic passphrase beta"}`))
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("refusal issued session")
		}
		if capped {
			if w.Code != 401 || !strings.Contains(w.Body.String(), "reauth_failed") {
				t.Fatal("change bypassed shared counter")
			}
		} else {
			var response struct {
				Factor bool `json:"factor"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 403 || !response.Factor {
				t.Fatal("factor did not fail closed", err)
			}
		}
	}
}

type uncertainFlow struct{ FlowStore }

func (uncertainFlow) LookupLink(context.Context, string) (accountflow.Link, error) {
	return accountflow.Link{Purpose: "reset", Email: "person@example.test"}, nil
}
func (uncertainFlow) CompleteReset(context.Context, accountflow.Completion) (int64, error) {
	return 42, context.DeadlineExceeded
}
func (uncertainFlow) CompleteRegistration(context.Context, accountflow.Completion) (store.CreateSessionRow, error) {
	return store.CreateSessionRow{AccountID: 42}, context.DeadlineExceeded
}
func TestCompletion_UncertainCommitEvictsAuthority(t *testing.T) {
	for _, reset := range []bool{false, true} {
		var evicted int64
		h := Handler{Flows: uncertainFlow{}, PublicURL: "https://example.test", Sessions: &websession.Manager{}, HashPassword: func(context.Context, string) (string, error) { return "new", nil }, Evict: func(id int64) { evicted = id }}
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"token":"synthetic","password":"synthetic passphrase alpha"}`))
		req.Header.Set("Origin", h.PublicURL)
		req.Header.Set("Content-Type", "application/json")
		method := h.SignupComplete
		if reset {
			method = h.ResetComplete
		}
		w := httptest.NewRecorder()
		method(w, req)
		if w.Code != 503 || len(w.Result().Cookies()) != 0 || evicted != 42 {
			t.Fatalf("uncertain commit status=%d evicted=%d", w.Code, evicted)
		}
	}
}

func TestPasswordChange_RejectsNullAndMixedShapes(t *testing.T) {
	for _, body := range []string{
		`{"token":null,"current":"synthetic passphrase alpha","password":"synthetic passphrase beta"}`,
		`{"token":"synthetic","current":null,"password":"synthetic passphrase beta"}`,
		`{"current":"synthetic passphrase alpha","password":"synthetic passphrase beta","code":null}`,
	} {
		sessions := &websession.Manager{Store: sessionStore{}, PublicURL: "https://example.test"}
		h := Handler{Proofs: &proofStore{}, Flows: &mailFlow{}, HashPassword: func(context.Context, string) (string, error) { return "", context.DeadlineExceeded }}
		w := httptest.NewRecorder()
		sessions.Middleware(sessions.CSRF(http.HandlerFunc(h.Password))).ServeHTTP(w, reauthRequest(body))
		if w.Code != 400 {
			t.Errorf("shape status=%d want=400", w.Code)
		}
	}
}

func TestResetCompletion_RejectsNext(t *testing.T) {
	h := Handler{PublicURL: "https://example.test"}
	r := httptest.NewRequest("POST", "/api/v1/auth/reset/complete", strings.NewReader(`{"token":"synthetic","password":"short","next":"/"}`))
	r.Header.Set("Origin", h.PublicURL)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ResetComplete(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_request") {
		t.Fatal("reset accepted the signup-only next field")
	}
}
