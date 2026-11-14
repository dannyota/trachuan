package passwordauth

import (
	"context"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type ProofStore interface {
	GetAccount(context.Context, int64) (store.Account, error)
	ReauthPassword(context.Context, store.PasswordSnapshot, int64) error
}

func (h *Handler) Reauth(w http.ResponseWriter, r *http.Request) {
	principal, ok := websession.FromContext(r.Context())
	if !ok {
		apierror.Write(w, 401, "reauth_failed")
		return
	}
	var body struct {
		Password *string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Password == nil {
		apierror.Write(w, 400, "invalid_request")
		return
	}
	query, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	account, err := h.Proofs.GetAccount(query, principal.AccountID)
	cancel()
	if h.failure(w, err, "reauth_failed") {
		return
	}
	verified, match, err := h.verify(r.Context(), account.Email, *body.Password)
	if h.failure(w, err, "reauth_failed") {
		return
	}
	if !match || verified.AccountID != principal.AccountID || verified.AuthEpoch != principal.AuthEpoch {
		apierror.Write(w, 401, "reauth_failed")
		return
	}
	query, cancel = context.WithTimeout(r.Context(), 2*time.Second)
	err = h.Proofs.ReauthPassword(query, verified, principal.SessionID)
	cancel()
	if h.failure(w, err, "reauth_failed") {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(204)
}
