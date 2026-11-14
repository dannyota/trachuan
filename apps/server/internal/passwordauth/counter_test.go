package passwordauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

func counterHandler(hasher Hasher) *Handler {
	hash, version := "synthetic", int64(1)
	return &Handler{Store: fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}, Hasher: hasher, Counter: password.NewCounter(time.Now), Sessions: &fakeSessions{}, Proofs: &proofStore{}, PublicURL: "https://example.test"}
}

func counterLogin(h *Handler) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{"email":"person@EXAMPLE.TEST","password":"synthetic passphrase"}`))
	r.Header.Set("Origin", h.PublicURL)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r)
	return w
}

func TestLogin_SuccessesDoNotConsumeFailureBudget(t *testing.T) {
	h := counterHandler(&fakeHasher{match: true})
	for i := range 11 {
		if w := counterLogin(h); w.Code != 200 {
			t.Fatalf("successful sign-in %d returned %d", i+1, w.Code)
		}
	}
}

func TestLogin_FailuresRemainAfterSuccess(t *testing.T) {
	hasher := &fakeHasher{}
	h := counterHandler(hasher)
	for range 9 {
		if w := counterLogin(h); w.Code != 401 {
			t.Fatal("wrong password admitted")
		}
	}
	hasher.match = true
	for range 2 {
		if w := counterLogin(h); w.Code != 200 {
			t.Fatal("success consumed a failure slot")
		}
	}
	hasher.match = false
	if w := counterLogin(h); w.Code != 401 {
		t.Fatal("tenth failure admitted")
	}
	hasher.match = true
	if w := counterLogin(h); w.Code != 401 || !strings.Contains(w.Body.String(), `"login_failed"`) {
		t.Fatal("correct password bypassed ten earlier failures")
	}
}

func TestLogin_OperationalErrorsDoNotConsumeFailureBudget(t *testing.T) {
	for _, scenario := range []string{"hash busy", "database"} {
		t.Run(scenario, func(t *testing.T) {
			hasher := &fakeHasher{match: true, err: password.ErrBusy}
			h := counterHandler(hasher)
			db := h.Store.(fakeStore)
			if scenario == "database" {
				db.err = context.DeadlineExceeded
				h.Store = db
			}
			for range 10 {
				if w := counterLogin(h); w.Code != 503 {
					t.Fatal("operational error did not fail closed")
				}
			}
			hasher.err = nil
			db.err = nil
			h.Store = db
			if w := counterLogin(h); w.Code != 200 {
				t.Fatal("operational errors consumed failure budget")
			}
		})
	}
}

func TestPassword_InvalidStoredHashDoesNotConsumeFailureBudget(t *testing.T) {
	hasher := password.NewHasher()
	repaired, err := hasher.Hash(t.Context(), "synthetic passphrase")
	if err != nil {
		t.Fatal(err)
	}
	for _, caller := range []string{"sign-in", "reauth", "change password"} {
		t.Run(caller, func(t *testing.T) {
			h := counterHandler(hasher)
			sessions := &websession.Manager{Store: sessionStore{}, PublicURL: h.PublicURL}
			h.Sessions = sessions
			h.HashPassword = func(context.Context, string) (string, error) { return repaired, nil }
			for range 10 {
				var w *httptest.ResponseRecorder
				switch caller {
				case "sign-in":
					w = counterLogin(h)
				case "reauth":
					w = httptest.NewRecorder()
					reauthFixture(h).ServeHTTP(w, reauthRequest(`{"password":"synthetic passphrase"}`))
				case "change password":
					w = httptest.NewRecorder()
					sessions.Middleware(sessions.CSRF(http.HandlerFunc(h.Password))).ServeHTTP(w, reauthRequest(`{"current":"synthetic passphrase","password":"synthetic replacement passphrase"}`))
				}
				if w.Code != 401 {
					t.Fatalf("invalid stored hash returned %d, want 401", w.Code)
				}
			}
			db := h.Store.(fakeStore)
			db.row.Hash = &repaired
			h.Store = db
			h.Sessions = &fakeSessions{}
			if w := counterLogin(h); w.Code != 200 {
				t.Fatalf("repaired credential returned %d, want 200", w.Code)
			}
		})
	}
}

func TestLogin_DummyFailuresConsumeBudget(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		h := counterHandler(&fakeHasher{match: true})
		db := h.Store.(fakeStore)
		known := db
		if unknown {
			db.err = pgx.ErrNoRows
		} else {
			db.row.Hash = nil
		}
		h.Store = db
		for range 10 {
			if w := counterLogin(h); w.Code != 401 {
				t.Fatal("dummy failure admitted")
			}
		}
		h.Store = known
		if w := counterLogin(h); w.Code != 401 {
			t.Fatal("dummy failures did not exhaust budget")
		}
	}
}

type heldHasher struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (h *heldHasher) Verify(context.Context, string, string) (bool, error) {
	h.calls.Add(1)
	h.entered <- struct{}{}
	<-h.release
	return false, nil
}
func (h *heldHasher) VerifyDummy(context.Context, string) error {
	h.entered <- struct{}{}
	return nil
}

func TestLogin_ConcurrentFailuresReserveAtMostTenVerifications(t *testing.T) {
	hasher := &heldHasher{entered: make(chan struct{}, 40), release: make(chan struct{})}
	h := counterHandler(hasher)
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			_, match, err := h.verify(t.Context(), "person@example.test", "synthetic passphrase")
			if match || err != nil {
				t.Error("failure changed answer")
			}
		})
	}
	for range 40 {
		<-hasher.entered
	}
	close(hasher.release)
	wg.Wait()
	if got := hasher.calls.Load(); got != 10 {
		t.Fatalf("admitted %d verifications, want 10", got)
	}
}

func TestPassword_CounterSharedAcrossAllCallers(t *testing.T) {
	hasher := &fakeHasher{}
	h := counterHandler(hasher)
	h.Sessions = &websession.Manager{Store: sessionStore{}, PublicURL: h.PublicURL}
	h.HashPassword = func(context.Context, string) (string, error) { return "synthetic new hash", nil }
	for range 4 {
		if w := counterLogin(h); w.Code != 401 {
			t.Fatal("wrong sign-in admitted")
		}
	}
	for range 3 {
		w := httptest.NewRecorder()
		reauthFixture(h).ServeHTTP(w, reauthRequest(`{"password":"synthetic wrong passphrase"}`))
		if w.Code != 401 {
			t.Fatal("wrong reauth admitted")
		}
	}
	change := h.Sessions.(*websession.Manager)
	for range 3 {
		w := httptest.NewRecorder()
		change.Middleware(change.CSRF(http.HandlerFunc(h.Password))).ServeHTTP(w, reauthRequest(`{"current":"synthetic wrong passphrase","password":"synthetic new passphrase"}`))
		if w.Code != 401 {
			t.Fatal("wrong current password admitted")
		}
	}
	hasher.match = true
	if w := counterLogin(h); w.Code != 401 {
		t.Fatal("sign-in bypassed shared failures")
	}
	w := httptest.NewRecorder()
	reauthFixture(h).ServeHTTP(w, reauthRequest(`{"password":"synthetic passphrase"}`))
	if w.Code != 401 {
		t.Fatal("reauth bypassed shared failures")
	}
}
