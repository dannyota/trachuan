package passwordauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountemail"
	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"github.com/jackc/pgx/v5"
)

type Store interface {
	GetPasswordByEmail(context.Context, string) (store.PasswordSnapshot, error)
}
type Hasher interface {
	Verify(context.Context, string, string) (bool, error)
	VerifyDummy(context.Context, string) error
}
type Sessions interface {
	CreatePasswordSession(context.Context, http.ResponseWriter, *http.Request, store.PasswordSnapshot) error
}
type primarySessions interface {
	CreatePasswordLogin(context.Context, http.ResponseWriter, *http.Request, store.PasswordSnapshot, string) error
}
type Handler struct {
	Factors      FactorStore
	Enrollment   bool
	Flows        FlowStore
	HashPassword func(context.Context, string) (string, error)
	Evict        func(int64)
	Store        Store
	Hasher       Hasher
	Counter      *password.Counter
	Sessions     Sessions
	Proofs       ProofStore
	PublicURL    string
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	origins := r.Header.Values("Origin")
	if len(origins) != 1 || origins[0] != h.PublicURL {
		apierror.Write(w, 403, "csrf_rejected")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
		apierror.Write(w, 415, "unsupported_media_type")
		return
	}
	var body struct {
		Email    *string `json:"email"`
		Password *string `json:"password"`
		Next     string  `json:"next"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Email == nil || body.Password == nil {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	email, err := accountemail.Parse(*body.Email)
	parsed := err == nil
	if !parsed {
		email = ""
	}
	verified, match, err := h.verify(r.Context(), email, *body.Password)
	if h.failure(w, err, "login_failed") {
		return
	}
	if !parsed || !match {
		apierror.Write(w, 401, "login_failed")
		return
	}
	if sessions, ok := h.Sessions.(primarySessions); ok {
		err = sessions.CreatePasswordLogin(r.Context(), w, r, verified, weblogin.ValidateNext(body.Next))
	} else {
		err = h.Sessions.CreatePasswordSession(r.Context(), w, r, verified)
	}
	var pending *store.PendingRequired
	if errors.As(err, &pending) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"factor_required": true, "csrf": pending.CSRF})
		return
	}
	if h.failure(w, err, "login_failed") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"next": weblogin.ValidateNext(body.Next)})
}
func (h *Handler) verify(ctx context.Context, email, raw string) (store.PasswordSnapshot, bool, error) {
	admitted := h.Counter.Allow(email)
	query, cancel := context.WithTimeout(ctx, 2*time.Second)
	snapshot, err := h.Store.GetPasswordByEmail(query, email)
	cancel()
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return snapshot, false, err
	}
	normalized, policyErr := password.Normalize(raw)
	if policyErr != nil {
		normalized = "synthetic invalid password"
	}
	if !admitted || err != nil || snapshot.Hash == nil || snapshot.Version == nil || policyErr != nil {
		return snapshot, false, h.Hasher.VerifyDummy(ctx, normalized)
	}
	match, err := h.Hasher.Verify(ctx, *snapshot.Hash, normalized)
	if errors.Is(err, password.ErrHashInvalid) {
		return snapshot, false, h.Hasher.VerifyDummy(ctx, normalized)
	}
	return snapshot, match, err
}
func (h *Handler) failure(w http.ResponseWriter, err error, code string) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, password.ErrBusy):
		apierror.Write(w, 503, "busy")
	case errors.Is(err, store.ErrLoginFailed), errors.Is(err, store.ErrUnauthorized), errors.Is(err, pgx.ErrNoRows):
		apierror.Write(w, 401, code)
	default:
		apierror.Write(w, 503, "unavailable")
	}
	return true
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(out)
	if err == nil {
		var extra any
		if extraErr := decoder.Decode(&extra); extraErr != io.EOF {
			if extraErr != nil {
				err = extraErr
			} else {
				err = errors.New("extra JSON")
			}
		}
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			apierror.Write(w, 413, "body_too_large")
		} else {
			apierror.Write(w, 400, "invalid_request")
		}
		return false
	}
	return true
}
