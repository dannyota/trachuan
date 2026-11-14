package weblogin

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"net/url"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	setLoginCookie(w, "", -1)
	cookies := r.CookiesNamed(loginCookie)
	if len(cookies) != 1 || !validHandle(cookies[0].Value) {
		rejection(w, r, "login_expired")
		return
	}
	tx, err := h.store.ClaimLoginTransaction(r.Context(), ClaimLoginTransactionParams{HandleHash: hashHandle(cookies[0].Value), Now: h.clock()})
	if errors.Is(err, sql.ErrNoRows) {
		rejection(w, r, "login_expired")
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	states, codes := q["state"], q["code"]
	if err != nil || len(states) != 1 || states[0] == "" || subtle.ConstantTimeCompare([]byte(states[0]), []byte(tx.State)) != 1 {
		rejection(w, r, "login_failed")
		return
	}
	if providerErrors, ok := q["error"]; ok {
		if len(providerErrors) == 1 && (providerErrors[0] == "server_error" || providerErrors[0] == "temporarily_unavailable") {
			unavailable(w)
		} else {
			rejection(w, r, "login_failed")
		}
		return
	}
	if len(codes) != 1 || codes[0] == "" {
		rejection(w, r, "login_failed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), providerTimeout)
	defer cancel()
	ctx = oidc.ClientContext(ctx, h.client)
	cfg := h.oauth
	cfg.RedirectURL = tx.RedirectURI
	token, err := cfg.Exchange(ctx, codes[0], oauth2.VerifierOption(tx.PKCEVerifier))
	if err != nil {
		if rejectedExchange(err) {
			rejection(w, r, "login_failed")
		} else {
			unavailable(w)
		}
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		rejection(w, r, "login_failed")
		return
	}
	id, available := h.verify(ctx, raw)
	if !available {
		unavailable(w)
		return
	}
	if id == nil || id.Subject == "" || (id.Issuer != h.issuer && (h.issuer != googleIssuer || id.Issuer != "accounts.google.com")) || !id.Expiry.After(h.clock()) || id.Nonce == "" ||
		subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(tx.Nonce)) != 1 {
		rejection(w, r, "login_failed")
		return
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err = id.Claims(&claims); err != nil || !claims.EmailVerified || claims.Email == "" {
		rejection(w, r, "login_failed")
		return
	}
	staged := &sessionHeaders{header: make(http.Header)}
	next := ValidateNext(tx.NextPath)
	err = h.sessions.CreateGoogleSession(ctx, staged, r, store.GoogleIdentity{Sub: id.Subject, Email: claims.Email, SignupEmails: h.signupEmails, Next: next, PendingHash: tx.PendingHash})
	var pending *store.PendingRequired
	if errors.As(err, &pending) {
		next = "/login/factor"
	} else if err != nil {
		switch {
		case errors.Is(err, store.ErrLoginFailed):
			rejection(w, r, "login_failed")
		case errors.Is(err, store.ErrSignupClosed):
			rejection(w, r, "signup_closed")
		case errors.Is(err, store.ErrAccountBlocked):
			rejection(w, r, "account_blocked")
		default:
			unavailable(w)
		}
		return
	}
	for key, values := range staged.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func rejectedExchange(err error) bool {
	var response *oauth2.RetrieveError
	if !errors.As(err, &response) || response.Response == nil || response.Response.StatusCode != http.StatusBadRequest {
		return false
	}
	switch response.ErrorCode {
	case "invalid_grant", "invalid_request", "access_denied":
		return true
	default:
		return false
	}
}

func (h *Handler) verify(ctx context.Context, raw string) (*oidc.IDToken, bool) {
	unavailable := false
	ctx = context.WithValue(ctx, keyFailureContextKey{}, &unavailable)
	id, err := h.verifier.Verify(ctx, raw)
	if unavailable || ctx.Err() != nil {
		return nil, false
	}
	if err != nil {
		return nil, true
	}
	return id, true
}

type sessionHeaders struct{ header http.Header }

func (s *sessionHeaders) Header() http.Header       { return s.header }
func (*sessionHeaders) Write(p []byte) (int, error) { return len(p), nil }
func (*sessionHeaders) WriteHeader(int)             {}
