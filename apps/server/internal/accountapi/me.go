package accountapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

type Store interface {
	GetAccount(context.Context, int64) (store.Account, error)
	GetPasswordCredential(context.Context, int64) (store.PasswordCredential, error)
	HasTOTP(context.Context, int64) (bool, error)
	ListLiveSessions(context.Context, int64) ([]store.WebSession, error)
	ListTokens(context.Context, int64) ([]store.PersonalToken, error)
	CreateWebPersonalToken(context.Context, store.WebPersonalTokenParams) (store.PersonalToken, error)
	RevokeOwnToken(context.Context, store.RevokeOwnTokenParams) (string, error)
	RequestDeletion(context.Context, store.RequestDeletionParams) (store.RequestDeletionRow, error)
}

type Sessions interface {
	EvictAccount(int64)
	RevokeOwnSession(context.Context, int64, int64) (int64, error)
}

type Verifier interface {
	Evict(string)
	EvictAccount(int64)
}

type Limiter interface {
	Allow(int64, limits.Class) (bool, time.Duration)
}

type Config struct {
	TOTPEnrollment bool
	Store          Store
	Sessions       Sessions
	Verifier       Verifier
	Limiter        Limiter
	Clock          func() time.Time
	Rand           io.Reader
}

type Handler struct {
	cfg      Config
	mux      *http.ServeMux
	randomMu sync.Mutex
}

func New(cfg Config) *Handler {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Reader
	}
	h := &Handler{cfg: cfg, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /api/v1/me", h.me)
	h.mux.HandleFunc("DELETE /api/v1/me", h.deleteAccount)
	h.mux.HandleFunc("GET /api/v1/me/sessions", h.listSessions)
	h.mux.HandleFunc("DELETE /api/v1/me/sessions/{id}", h.revokeSession)
	h.mux.HandleFunc("GET /api/v1/me/grants", h.listGrants)
	h.mux.HandleFunc("DELETE /api/v1/me/grants/{id}", h.revokeGrant)
	h.mux.HandleFunc("GET /api/v1/me/tokens", h.listTokens)
	h.mux.HandleFunc("POST /api/v1/me/tokens", h.createToken)
	h.mux.HandleFunc("DELETE /api/v1/me/tokens/{id}", h.revokeToken)
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, ok := websession.FromContext(r.Context())
	if !ok {
		apierror.Write(w, 401, "unauthenticated")
		return
	}
	if h.cfg.Store == nil || h.cfg.Sessions == nil || h.cfg.Verifier == nil || h.cfg.Limiter == nil {
		apierror.Write(w, 503, "unavailable")
		return
	}
	if ok, wait := h.cfg.Limiter.Allow(p.AccountID, limits.Read); !ok {
		apierror.RateLimited(w, wait)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	h.mux.ServeHTTP(w, r.WithContext(ctx))
}
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, _ := websession.FromContext(r.Context())
	a, err := h.cfg.Store.GetAccount(r.Context(), p.AccountID)
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	_, passwordErr := h.cfg.Store.GetPasswordCredential(r.Context(), p.AccountID)
	if passwordErr != nil && !errors.Is(passwordErr, pgx.ErrNoRows) {
		unavailable(w)
		return
	}
	totp, err := h.cfg.Store.HasTOTP(r.Context(), p.AccountID)
	if err != nil || r.Context().Err() != nil {
		unavailable(w)
		return
	}
	writeJSON(w, 200, struct {
		Password       bool      `json:"password"`
		Google         bool      `json:"google"`
		TOTP           bool      `json:"totp"`
		TOTPEnrollment bool      `json:"totp_enrollment"`
		ID             int64     `json:"id"`
		Email          string    `json:"email"`
		CreatedAt      time.Time `json:"created_at"`
		CSRFToken      string    `json:"csrf_token"`
	}{passwordErr == nil, a.GoogleSub != nil, totp, h.cfg.TOTPEnrollment, a.ID, a.Email, a.CreatedAt, p.CSRFSecret})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func unavailable(w http.ResponseWriter) { apierror.Write(w, 503, "unavailable") }
func notFound(w http.ResponseWriter)    { apierror.Write(w, 404, "not_found") }
func invalid(w http.ResponseWriter)     { apierror.Write(w, 400, "invalid_request") }
func idParam(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}
func storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w)
	} else {
		unavailable(w)
	}
}
