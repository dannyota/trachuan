package passwordauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountemail"
	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type FlowStore interface {
	RequestMail(context.Context, string, string) error
	LookupLink(context.Context, string) (accountflow.Link, error)
	CompleteRegistration(context.Context, accountflow.Completion) (store.CreateSessionRow, error)
	CompleteReset(context.Context, accountflow.Completion) (int64, error)
	RequestAddPassword(context.Context, accountflow.Acting) error
	ChangePassword(context.Context, accountflow.Completion) (store.CreateSessionRow, error)
}
type flowSessions interface {
	CommitSession(context.Context, http.ResponseWriter, *http.Request, func(context.Context, store.CreateSessionParams) (store.CreateSessionRow, error)) error
}

func (h *Handler) preSession(w http.ResponseWriter, r *http.Request) bool {
	if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != h.PublicURL {
		apierror.Write(w, 403, "csrf_rejected")
		return false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
		apierror.Write(w, 415, "unsupported_media_type")
		return false
	}
	return true
}
func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) { h.requestMail(w, r, "registration") }
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request)  { h.requestMail(w, r, "reset") }
func (h *Handler) requestMail(w http.ResponseWriter, r *http.Request, purpose string) {
	if !h.preSession(w, r) {
		return
	}
	var body struct {
		Email *string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Email == nil {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	email, err := accountemail.Parse(*body.Email)
	if err != nil {
		email = ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.flowFailure(w, h.Flows.RequestMail(ctx, purpose, email)) {
		return
	}
	emptyJSON(w, 202)
}
func (h *Handler) Link(w http.ResponseWriter, r *http.Request) {
	if !h.preSession(w, r) {
		return
	}
	var body struct {
		Token *string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Token == nil {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	link, err := h.Flows.LookupLink(ctx, linkDigest(*body.Token))
	if h.flowFailure(w, err) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"purpose": link.Purpose, "email": link.Email})
}
func (h *Handler) SignupComplete(w http.ResponseWriter, r *http.Request) { h.completeLink(w, r, false) }
func (h *Handler) ResetComplete(w http.ResponseWriter, r *http.Request)  { h.completeLink(w, r, true) }
func (h *Handler) completeLink(w http.ResponseWriter, r *http.Request, reset bool) {
	if !h.preSession(w, r) {
		return
	}
	var body struct {
		Token    *string         `json:"token"`
		Password *string         `json:"password"`
		Next     json.RawMessage `json:"next"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Token == nil || body.Password == nil || reset && body.Next != nil {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	digest := linkDigest(*body.Token)
	hash, ok := h.newPassword(w, r, *body.Password, digest, "")
	if !ok {
		return
	}
	p := accountflow.Completion{TokenHash: digest, Hash: hash}
	if reset {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		id, err := h.Flows.CompleteReset(ctx, p)
		if id != 0 {
			h.evict(id)
		}
		if h.flowFailure(w, err) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(204)
		return
	}
	h.commit(w, r, func(ctx context.Context, params store.CreateSessionParams) (store.CreateSessionRow, error) {
		p.Session = params
		return h.Flows.CompleteRegistration(ctx, p)
	})
}
func (h *Handler) newPassword(w http.ResponseWriter, r *http.Request, raw, digest, email string) (string, bool) {
	normalized, err := password.Check(raw, email)
	if err != nil {
		apierror.Write(w, 400, "password_rejected")
		return "", false
	}
	if digest != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		link, lookupErr := h.Flows.LookupLink(ctx, digest)
		cancel()
		if h.flowFailure(w, lookupErr) {
			return "", false
		}
		normalized, err = password.Check(raw, link.Email)
		if err != nil {
			apierror.Write(w, 400, "password_rejected")
			return "", false
		}
	}
	hash, err := h.HashPassword(r.Context(), normalized)
	if h.flowFailure(w, err) {
		return "", false
	}
	return hash, true
}
func (h *Handler) commit(w http.ResponseWriter, r *http.Request, write func(context.Context, store.CreateSessionParams) (store.CreateSessionRow, error)) {
	sessions, ok := h.Sessions.(flowSessions)
	if !ok {
		apierror.Write(w, 503, "unavailable")
		return
	}
	err := sessions.CommitSession(r.Context(), w, r, func(ctx context.Context, p store.CreateSessionParams) (store.CreateSessionRow, error) {
		row, err := write(ctx, p)
		if row.AccountID != 0 {
			h.evict(row.AccountID)
		}
		return row, err
	})
	if h.flowFailure(w, err) {
		return
	}
	emptyJSON(w, 200)
}
func (h *Handler) evict(id int64) {
	if h.Evict != nil {
		h.Evict(id)
	}
}
func linkDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func emptyJSON(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("{}\n"))
}
func (h *Handler) flowFailure(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, accountflow.ErrLinkInvalid):
		apierror.Write(w, 409, "link_invalid")
	case errors.Is(err, accountflow.ErrInvalidRequest):
		apierror.Write(w, 400, "invalid_request")
	case errors.Is(err, accountflow.ErrRateLimited):
		apierror.RateLimited(w, 24*time.Hour)
	case errors.Is(err, store.ErrReauthRequired):
		apierror.Write(w, 403, "reauth_required")
	case errors.Is(err, store.ErrFactorRequired):
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(403)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "reauth_required", "message": apierror.Message("reauth_required")}, "factor": true})
	case errors.Is(err, store.ErrUnauthorized):
		apierror.Write(w, 401, "unauthenticated")
	default:
		h.failure(w, err, "reauth_failed")
	}
	return true
}
func acting(r *http.Request) (accountflow.Acting, bool) {
	p, ok := websession.FromContext(r.Context())
	return accountflow.Acting{AccountID: p.AccountID, SessionID: p.SessionID, AuthEpoch: p.AuthEpoch}, ok
}
