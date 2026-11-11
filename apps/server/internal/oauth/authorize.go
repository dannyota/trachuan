package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

// NewAuthorizeHandler validates requests and stores browser-bound consent.
// The caller supplies the shared login and OAuth IP limiter.
func NewAuthorizeHandler(base string, db ClientDB, resolve func(context.Context, string) (Client, error)) http.Handler {
	pages := newConsentPages(base)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			pages.failure(w, r, 405, "invalid_request", "", "")
			return
		}
		values, parseErr := url.ParseQuery(r.URL.RawQuery)
		row, code, target := authorizeRequest(r.Context(), base, values, parseErr, resolve)
		if code != "" {
			pages.failure(w, r, oauthPageStatus(code), code, target, row.State)
			return
		}
		handle, handleHash, err := NewSecret(rand.Reader, "")
		if err != nil {
			pages.failure(w, r, 503, "temporarily_unavailable", target, row.State)
			return
		}
		browser, browserHash, err := NewSecret(rand.Reader, "")
		if err != nil {
			pages.failure(w, r, 503, "temporarily_unavailable", target, row.State)
			return
		}
		row.HandleHash = fmt.Sprintf("%x", handleHash)
		row.BrowserHash = fmt.Sprintf("%x", browserHash)
		row.ExpiresAt = time.Now().Add(10 * time.Minute)
		if err := storeConsentRequest(r.Context(), db, row); err != nil {
			code = "temporarily_unavailable"
			if errors.Is(err, ErrInvalidClient) {
				code = "invalid_client"
			}
			if errors.Is(err, consentUnauthorized) {
				code = "unauthorized_client"
			}
			pages.failure(w, r, oauthPageStatus(code), code, target, row.State)
			return
		}
		consentHeaders(w)
		http.SetCookie(w, oauthBrowserCookie(browser, 600))
		w.Header().Set("Location", "/oauth/consent/"+handle)
		w.WriteHeader(http.StatusSeeOther)
	})
}

func authorizeRequest(ctx context.Context, base string, v url.Values, parseErr error, resolve func(context.Context, string) (Client, error)) (store.OauthRequest, string, string) {
	row := store.OauthRequest{ClientID: v.Get("client_id"), RedirectUri: v.Get("redirect_uri"), State: v.Get("state"), CodeChallenge: v.Get("code_challenge"), Resource: v.Get("resource")}
	if len(v["state"]) != 1 || len(row.State) > 512 {
		row.State = ""
	}
	if len(v["client_id"]) != 1 || row.ClientID == "" {
		return row, "invalid_client", ""
	}
	client, err := resolve(ctx, row.ClientID)
	if err != nil {
		if errors.Is(err, ErrInvalidClient) {
			return row, "invalid_client", ""
		}
		return row, "temporarily_unavailable", ""
	}
	target := ""
	if len(v["redirect_uri"]) == 1 && consentRedirectMatches(client, row.RedirectUri) {
		target = row.RedirectUri
	}
	if target == "" {
		return row, "invalid_request", ""
	}
	if !client.Allows("authorization_code") {
		return row, "unauthorized_client", target
	}
	if parseErr != nil {
		return row, "invalid_request", target
	}
	for _, key := range []string{"client_id", "redirect_uri", "response_type", "code_challenge", "code_challenge_method", "state", "resource", "scope"} {
		if len(v[key]) > 1 {
			return row, "invalid_request", target
		}
	}
	if len(v.Get("state")) > 512 {
		row.State = ""
		return row, "invalid_request", target
	}
	if v.Get("response_type") != "code" {
		return row, "unsupported_response_type", target
	}
	if v.Get("code_challenge_method") != "S256" || !authorizeChallenge(row.CodeChallenge) {
		return row, "invalid_request", target
	}
	if row.Resource != base+"/mcp" {
		return row, "invalid_target", target
	}
	row.Scope, err = ParseScope(v.Get("scope"))
	if err != nil {
		return row, "invalid_scope", target
	}
	return row, "", target
}

func authorizeChallenge(raw string) bool {
	if len(raw) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	return err == nil && len(decoded) == 32
}

func consentRedirectMatches(client Client, redirect string) bool {
	for _, registered := range client.RedirectURIs {
		if MatchRedirect(registered, redirect) {
			return true
		}
	}
	return false
}

func storeConsentRequest(ctx context.Context, db ClientDB, row store.OauthRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin pending consent: %w", err)
	}
	defer consentRollback(tx)
	client, err := InUse(ctx, tx, row.ClientID)
	if err != nil {
		return fmt.Errorf("protect pending consent client: %w", err)
	}
	if !client.Allows("authorization_code") {
		return consentUnauthorized
	}
	// Serialize count and insert across processes without excluding client reads.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, ClientsLockKey+1); err != nil {
		return fmt.Errorf("lock pending consent capacity: %w", err)
	}
	q := store.New(tx)
	count, err := q.CountPendingRequests(ctx)
	if err != nil {
		return fmt.Errorf("count pending consent: %w", err)
	}
	if count >= 10000 {
		return ErrTemporarilyUnavailable
	}
	_, err = q.CreateRequest(ctx, store.CreateRequestParams{HandleHash: row.HandleHash, BrowserHash: row.BrowserHash, ClientID: row.ClientID, RedirectUri: row.RedirectUri, State: row.State, CodeChallenge: row.CodeChallenge, Resource: row.Resource, Scope: row.Scope, ExpiresAt: row.ExpiresAt})
	if err != nil {
		return fmt.Errorf("store pending consent: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pending consent: %w", err)
	}
	return nil
}

func consentRollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func oauthBrowserCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: "__Host-oauth", Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge}
}

func oauthPageStatus(code string) int {
	if code == "temporarily_unavailable" {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}
