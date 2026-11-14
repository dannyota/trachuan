package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

type consentFailure string

func (e consentFailure) Error() string { return string(e) }

const (
	consentInvalid      consentFailure = "invalid_request"
	consentGrantLimit   consentFailure = "grant_limit"
	consentUnauthorized consentFailure = "unauthorized_client"
)

func NewConsentHandler(base string, db ClientDB, sessions *websession.Manager) http.Handler {
	pages := newConsentPages(base)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			pages.failure(w, r, 405, "invalid_request", "", "")
			return
		}
		handle := strings.TrimPrefix(r.URL.Path, "/oauth/consent/")
		browser, valid := consentBrowser(r)
		if !strings.HasPrefix(r.URL.Path, "/oauth/consent/") || !authorizeChallenge(handle) || !valid {
			pages.failure(w, r, 400, "invalid_request", "", "")
			return
		}
		row, err := readConsentRequest(r.Context(), db, handle, browser)
		if err != nil {
			code := "temporarily_unavailable"
			if errors.Is(err, pgx.ErrNoRows) {
				code = "invalid_request"
			}
			pages.failure(w, r, oauthPageStatus(code), code, "", "")
			return
		}
		authenticated, status := consentSession(w, r, sessions)
		if status != 0 {
			if status == 401 {
				consentHeaders(w)
				w.Header().Set("Location", "/api/v1/auth/google/start?next="+url.QueryEscape("/oauth/consent/"+handle))
				w.WriteHeader(http.StatusSeeOther)
			} else {
				pages.failure(w, r, 503, "temporarily_unavailable", row.RedirectUri, row.State)
			}
			return
		}
		r = authenticated
		principal, _ := websession.FromContext(r.Context())
		if r.Method == http.MethodGet {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			client, clientErr := store.New(db).GetClient(ctx, row.ClientID)
			if clientErr != nil {
				code := "temporarily_unavailable"
				if errors.Is(clientErr, pgx.ErrNoRows) {
					code = "invalid_client"
				}
				pages.failure(w, r, oauthPageStatus(code), code, "", "")
				return
			}
			pages.consent(w, r, clientFromRow(client), row, handle, consentFormToken(principal.CSRFSecret, handle))
			return
		}
		decision, ok := consentDecision(w, r, base, handle, principal.CSRFSecret)
		if !ok {
			pages.failure(w, r, 400, "invalid_request", row.RedirectUri, row.State)
			return
		}
		code, err := decideConsent(r.Context(), db, row, handle, browser, principal, decision)
		if err != nil {
			errorCode := "temporarily_unavailable"
			if errors.Is(err, consentInvalid) || errors.Is(err, pgx.ErrNoRows) {
				errorCode = "invalid_request"
			}
			if errors.Is(err, ErrInvalidClient) {
				errorCode = "invalid_client"
			}
			if errors.Is(err, consentGrantLimit) {
				errorCode = "grant_limit"
			}
			if errors.Is(err, consentUnauthorized) {
				errorCode = "unauthorized_client"
			}
			pages.failure(w, r, oauthPageStatus(errorCode), errorCode, row.RedirectUri, row.State)
			return
		}
		resultError := ""
		if decision == "deny" {
			resultError = "access_denied"
		}
		consentHeaders(w)
		http.SetCookie(w, oauthBrowserCookie("", -1))
		w.Header().Set("Location", consentResultURL(row.RedirectUri, base, code, resultError, row.State))
		w.WriteHeader(http.StatusSeeOther)
	})
}

func consentBrowser(r *http.Request) (string, bool) {
	values := r.CookiesNamed("__Host-oauth")
	if len(values) != 1 || !authorizeChallenge(values[0].Value) {
		return "", false
	}
	return values[0].Value, true
}

func readConsentRequest(ctx context.Context, db store.DBTX, handle, browser string) (store.OauthRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var row store.OauthRequest
	err := db.QueryRow(ctx, `SELECT client_id, redirect_uri, state, code_challenge, resource, scope
 FROM oauth_request WHERE handle_hash=$1 AND browser_hash=$2
 AND claimed_at IS NULL AND expires_at > now()`, consentDigest(handle), consentDigest(browser)).Scan(&row.ClientID, &row.RedirectUri, &row.State, &row.CodeChallenge, &row.Resource, &row.Scope)
	if err != nil {
		return row, fmt.Errorf("read pending consent: %w", err)
	}
	return row, nil
}

func consentSession(w http.ResponseWriter, r *http.Request, sessions *websession.Manager) (*http.Request, int) {
	if sessions == nil {
		return nil, 401
	}
	probe := &consentSessionProbe{ResponseWriter: w}
	var authenticated *http.Request
	sessions.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { authenticated = request })).ServeHTTP(probe, r)
	if authenticated == nil {
		return nil, probe.status
	}
	return authenticated, 0
}

type consentSessionProbe struct {
	http.ResponseWriter
	status int
}

func (p *consentSessionProbe) WriteHeader(status int)         { p.status = status }
func (p *consentSessionProbe) Write(body []byte) (int, error) { return len(body), nil }

func consentFormToken(secret, handle string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("oauth-consent:" + handle))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func consentDigest(raw string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) }

func consentDecision(w http.ResponseWriter, r *http.Request, base, handle, secret string) (string, bool) {
	if len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != base || len(r.Header.Values("Content-Type")) != 1 || secret == "" {
		return "", false
	}
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/x-www-form-urlencoded" {
		return "", false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err = r.ParseForm(); err != nil {
		return "", false
	}
	for _, values := range r.PostForm {
		if len(values) != 1 {
			return "", false
		}
	}
	token := r.PostForm.Get("csrf_token")
	if !hmac.Equal([]byte(token), []byte(consentFormToken(secret, handle))) {
		return "", false
	}
	decision := r.PostForm.Get("decision")
	return decision, decision == "approve" || decision == "deny"
}

func decideConsent(ctx context.Context, db ClientDB, pending store.OauthRequest, handle, browser string, principal websession.Principal, decision string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin consent decision: %w", err)
	}
	defer consentRollback(tx)
	q := store.New(tx)
	accountID := principal.AccountID
	account, err := store.LockAccount(ctx, tx, accountID)
	if err != nil {
		return "", fmt.Errorf("lock consent account: %w", err)
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil || account.AuthEpoch != principal.AuthEpoch {
		return "", consentInvalid
	}
	_, err = q.GetLiveAuthoritySession(ctx, store.GetLiveAuthoritySessionParams{ID: principal.SessionID, AccountID: accountID, AuthEpoch: account.AuthEpoch})
	if err != nil {
		return "", fmt.Errorf("recheck consent session: %w", err)
	}

	client, err := InUse(ctx, tx, pending.ClientID)
	if err != nil {
		return "", fmt.Errorf("protect consent client: %w", err)
	}
	if decision == "approve" && !client.Allows("authorization_code") {
		return "", consentUnauthorized
	}

	row, err := q.ClaimRequest(ctx, store.ClaimRequestParams{HandleHash: consentDigest(handle), BrowserHash: consentDigest(browser), AccountID: &accountID})
	if err != nil {
		return "", fmt.Errorf("claim consent request: %w", err)
	}
	code := ""
	if decision == "approve" {
		grant, grantErr := q.UpsertGrant(ctx, store.UpsertGrantParams{AccountID: accountID, ClientID: row.ClientID, Resource: row.Resource, Scope: row.Scope})
		if grantErr != nil {
			return "", fmt.Errorf("upsert consent grant: %w", grantErr)
		}
		count, countErr := q.CountLiveGrants(ctx, accountID)
		if countErr != nil {
			return "", fmt.Errorf("count consent grants: %w", countErr)
		}
		if count > 20 {
			return "", consentGrantLimit
		}
		var digest [32]byte
		code, digest, err = NewSecret(rand.Reader, "")
		if err != nil {
			return "", fmt.Errorf("create consent code entropy: %w", err)
		}
		_, err = q.CreateCode(ctx, store.CreateCodeParams{CodeHash: fmt.Sprintf("%x", digest), GrantID: grant.ID, RedirectUri: row.RedirectUri, Resource: row.Resource, CodeChallenge: row.CodeChallenge, Scope: row.Scope, AuthEpoch: account.AuthEpoch, ExpiresAt: time.Now().Add(time.Minute)})
		if err != nil {
			return "", fmt.Errorf("store consent code: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit consent decision: %w", err)
	}
	return code, nil
}

func consentResultURL(redirect, issuer, code, errorCode, state string) string {
	u, err := url.Parse(redirect)
	if err != nil {
		return ""
	}
	values := u.Query()
	for _, key := range []string{"code", "error", "error_description", "error_uri", "state", "iss"} {
		values.Del(key)
	}
	if code != "" {
		values.Set("code", code)
	}
	if errorCode != "" {
		values.Set("error", errorCode)
	}
	values.Set("state", state)
	values.Set("iss", issuer)
	u.RawQuery = values.Encode()
	return u.String()
}
