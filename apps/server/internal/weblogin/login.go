package weblogin

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const googleIssuer = "https://accounts.google.com"
const loginCookie = "__Host-login"
const transactionTTL = 10 * time.Minute
const providerTimeout = 10 * time.Second

// SessionCreator revokes a presented session before issuing a new one. Errors
// must leave no new session in storage. Callback discards response headers on error.
type SessionCreator interface {
	CreateSession(context.Context, http.ResponseWriter, *http.Request, int64) error
}

// Config supplies credentials and login dependencies. Issuer overrides Google
// only in tests. Clock and Rand default to time.Now and crypto/rand.Reader.
type Config struct {
	ClientID, ClientSecret, PublicURL string
	Issuer                            string
	Store                             Store
	Sessions                          SessionCreator
	Clock                             func() time.Time
	Rand                              io.Reader
}

// Handler exposes GET-only Google authorization start and callback routes.
type Handler struct {
	Start, Callback http.Handler
	oauth           oauth2.Config
	provider        *oidc.Provider
	issuer          string
	store           Store
	sessions        SessionCreator
	clock           func() time.Time
	random          io.Reader
	randomMu        sync.Mutex
	client          *http.Client
}

// New discovers the provider once at startup. An HTTP client supplied through
// oidc.ClientContext is retained, with a maximum ten-second request timeout.
func New(ctx context.Context, cfg Config) (*Handler, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.Store == nil || cfg.Sessions == nil {
		return nil, errors.New("weblogin: client credentials, store and sessions are required")
	}
	public, err := url.Parse(cfg.PublicURL)
	if err != nil || public.Host == "" || (public.Scheme != "https" && public.Scheme != "http") || public.User != nil ||
		(public.Path != "" && public.Path != "/") || public.RawQuery != "" || public.ForceQuery || public.Fragment != "" || strings.Contains(cfg.PublicURL, "#") {
		return nil, errors.New("weblogin: public URL must be an HTTP or HTTPS origin")
	}
	if cfg.Issuer == "" {
		cfg.Issuer = googleIssuer
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Reader
	}
	client := *http.DefaultClient
	if supplied, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		client = *supplied
	}
	if client.Timeout == 0 || client.Timeout > providerTimeout {
		client.Timeout = providerTimeout
	}
	// Neither token exchange nor key discovery needs to follow a redirect.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, &client), cfg.Issuer)
	if err != nil {
		return nil, discoveryError{cause: err}
	}
	endpoint := provider.Endpoint()
	endpoint.AuthStyle = oauth2.AuthStyleInHeader
	h := &Handler{
		oauth:    oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: endpoint, RedirectURL: strings.TrimSuffix(cfg.PublicURL, "/") + "/api/v1/auth/google/callback", Scopes: []string{oidc.ScopeOpenID, "email"}},
		provider: provider, issuer: cfg.Issuer, store: cfg.Store, sessions: cfg.Sessions, clock: cfg.Clock, random: cfg.Rand, client: &client,
	}
	h.Start = http.HandlerFunc(h.start)
	h.Callback = http.HandlerFunc(h.callback)
	return h, nil
}

// Discovery errors may include provider response bodies. Keep startup logs fixed.
type discoveryError struct{ cause error }

func (e discoveryError) Error() string { return "weblogin: provider discovery unavailable" }
func (e discoveryError) Unwrap() error { return e.cause }

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	handle, tx, err := h.begin(r.Context(), validateNext(r.URL.Query().Get("next")))
	if err != nil {
		unavailable(w)
		return
	}
	setLoginCookie(w, handle, 600)
	http.Redirect(w, r, h.oauth.AuthCodeURL(tx.State, oauth2.S256ChallengeOption(tx.PKCEVerifier), oidc.Nonce(tx.Nonce)), http.StatusFound)
}

func setLoginCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: loginCookie, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func unavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(w, "{\"error\":{\"code\":\"unavailable\",\"message\":\"Service unavailable\"}}\n")
}

func getOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
}

func rejection(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+code, http.StatusFound)
}

func randomValue(reader io.Reader) (string, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", fmt.Errorf("generate login value: %w", err)
	}
	return encodeValue(value), nil
}
