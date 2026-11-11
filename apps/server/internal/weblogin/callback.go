package weblogin

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"

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
	if err != nil || len(states) != 1 || states[0] == "" || subtle.ConstantTimeCompare([]byte(states[0]), []byte(tx.State)) != 1 ||
		q.Has("error") || len(codes) != 1 || codes[0] == "" {
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
	if id == nil || id.Subject == "" || id.Issuer != h.issuer || !id.Expiry.After(h.clock()) || id.Nonce == "" ||
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
	account, err := h.store.UpsertGoogleAccount(ctx, UpsertGoogleAccountParams{Sub: id.Subject, Email: claims.Email})
	if err != nil {
		unavailable(w)
		return
	}
	if account.Blocked || account.DeleteRequested {
		rejection(w, r, "account_blocked")
		return
	}
	// Publish session headers only after the creator completes every database step.
	staged := &sessionHeaders{header: make(http.Header)}
	if err := h.sessions.CreateSession(ctx, staged, r, account.ID); err != nil {
		unavailable(w)
		return
	}
	for key, values := range staged.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	http.Redirect(w, r, validateNext(tx.NextPath), http.StatusFound)
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
	// Each verification gets its own failure recorder, so concurrent callbacks
	// cannot turn one another's signature rejection into an availability failure.
	transport := h.client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	keys := &keyTransport{base: transport}
	client := *h.client
	client.Transport = keys
	verifier := h.provider.VerifierContext(oidc.ClientContext(ctx, &client), &oidc.Config{ClientID: h.oauth.ClientID, Now: h.clock})
	id, err := verifier.Verify(ctx, raw)
	if keys.failed.Load() || ctx.Err() != nil {
		return nil, false
	}
	if err != nil {
		return nil, true
	}
	return id, true
}

type keyTransport struct {
	base   http.RoundTripper
	failed atomic.Bool
}

func (t *keyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil {
		t.failed.Store(true)
		return nil, fmt.Errorf("fetch provider keys: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		t.failed.Store(true)
		return response, nil
	}
	// Bound malformed endpoint responses and record read/JSON failures separately
	// from invalid token signatures. go-oidc does not expose typed fetch errors.
	data, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || len(data) > 1<<20 || !json.Valid(data) {
		t.failed.Store(true)
		return nil, errors.New("provider keys unavailable")
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, nil
}

type sessionHeaders struct{ header http.Header }

func (s *sessionHeaders) Header() http.Header       { return s.header }
func (*sessionHeaders) Write(p []byte) (int, error) { return len(p), nil }
func (*sessionHeaders) WriteHeader(int)             {}
