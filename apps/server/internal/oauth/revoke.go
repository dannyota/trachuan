package oauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type revokeHandler struct {
	db         TokenDB
	evict      func(string)
	evictGrant func(int64)
}

// NewRevokeHandler serves public-client token revocation. Callbacks evict local
// credential caches even when a commit has an unknown outcome. Mount the handler
// behind the shared login and OAuth IP limiter.
func NewRevokeHandler(db TokenDB, evict func(string), evictGrant func(int64)) http.Handler {
	return &revokeHandler{db: db, evict: evict, evictGrant: evictGrant}
}
func (h *revokeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(r.Header.Values("Authorization")) != 0 {
		writeTokenClientAuthError(w)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeTokenError(w, 405, "invalid_request")
		return
	}
	form, code := parseRevokeForm(r)
	if code != "" {
		if code == "invalid_client" {
			writeTokenClientAuthError(w)
		} else {
			writeTokenError(w, 400, code)
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.db == nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := store.New(tx)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(form.Get("token"))))
	token, err := q.GetToken(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		tokenHeaders(w)
		w.WriteHeader(200)
		return
	}
	if err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	if token.ClientID != form.Get("client_id") {
		writeTokenError(w, 400, "invalid_request")
		return
	}
	// Match the issuance lock order. A refresh cannot create tokens after the
	// grant revocation, including when it was waiting for the account lock.
	if _, err = q.LockCredentialAccount(ctx, token.AccountID); err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	if token.Kind == "refresh" {
		_, err = q.RevokeGrant(ctx, token.GrantID)
	} else {
		_, err = q.RevokeToken(ctx, hash)
	}
	if err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	err = tx.Commit(ctx)
	if token.Kind == "refresh" {
		if h.evictGrant != nil {
			h.evictGrant(token.GrantID)
		}
	} else if h.evict != nil {
		h.evict(hash)
	}
	if err != nil {
		writeTokenError(w, 503, "temporarily_unavailable")
		return
	}
	tokenHeaders(w)
	w.WriteHeader(200)
}
func parseRevokeForm(r *http.Request) (url.Values, string) {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/x-www-form-urlencoded" || len(r.Header.Values("Content-Type")) != 1 {
		return nil, "invalid_request"
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, "invalid_request"
	}
	form, err := url.ParseQuery(string(raw))
	if hasClientAuthenticationParameter(form) {
		return nil, "invalid_client"
	}
	if err != nil {
		return nil, "invalid_request"
	}
	for _, values := range form {
		if len(values) != 1 {
			return nil, "invalid_request"
		}
	}
	if form.Get("token") == "" || form.Get("client_id") == "" {
		return nil, "invalid_request"
	}
	return form, ""
}
