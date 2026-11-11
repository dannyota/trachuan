// Package oidctest provides an ephemeral OIDC issuer for login tests.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// ClientID is the test client's registered audience.
const ClientID = "test-client"

// Claims describes an ID token. Empty audience, issuer and expiry use defaults.
// EmailVerified is sent verbatim so tests can exercise missing and invalid types.
type Claims struct {
	Subject, Email, Nonce, Audience, Issuer string
	EmailVerified                           any
	ExpiresAt                               time.Time
	BadSignature                            bool
	Challenge, RedirectURI                  string
	OmitIDToken                             bool
}

// Provider serves discovery, JWKS and single-use, PKCE-bound token exchange.
type Provider struct {
	URL             string
	key             *rsa.PrivateKey
	t               *testing.T
	mu              sync.Mutex
	codes           map[string]Claims
	statuses        map[string]int
	counts          map[string]int
	lastForm        url.Values
	discoveryIssuer string
	brokenJWKS      bool
	autoLogin       bool
}

// New starts a provider and registers its cleanup with t.
func New(t *testing.T) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{key: key, t: t, codes: make(map[string]Claims), statuses: make(map[string]int), counts: make(map[string]int)}
	s := httptest.NewServer(http.HandlerFunc(p.serveHTTP))
	p.URL = s.URL
	t.Cleanup(s.Close)
	return p
}

// RegisterCode binds one code to claims and optional PKCE and redirect checks.
func (p *Provider) RegisterCode(code string, c Claims) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.codes[code] = c
}

// EnableLogin makes the authorization endpoint issue a test identity.
func (p *Provider) EnableLogin() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autoLogin = true
}

// SetStatus makes path return status; zero restores normal behavior.
func (p *Provider) SetStatus(path string, status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statuses[path] = status
}

// SetIssuer overrides the issuer in discovery metadata.
func (p *Provider) SetIssuer(issuer string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.discoveryIssuer = issuer
}

// SetBrokenJWKS makes the JWKS endpoint return structurally invalid JSON.
func (p *Provider) SetBrokenJWKS(broken bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.brokenJWKS = broken
}

// Count returns how often path was requested.
func (p *Provider) Count(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.counts[path]
}

// LastTokenForm returns a copy of the last token request's form body.
func (p *Provider) LastTokenForm() url.Values {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := make(url.Values)
	for k, vals := range p.lastForm {
		v[k] = append([]string(nil), vals...)
	}
	return v
}

func (p *Provider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.counts[r.URL.Path]++
	status := p.statuses[r.URL.Path]
	issuer := p.discoveryIssuer
	broken := p.brokenJWKS
	autoLogin := p.autoLogin
	p.mu.Unlock()
	if status != 0 {
		p.writeJSON(w, status, map[string]any{"error": "server_error", "error_description": "provider-private-detail"})
		return
	}
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		if issuer == "" {
			issuer = p.URL
		}
		p.writeJSON(w, http.StatusOK, map[string]any{
			"issuer": issuer, "authorization_endpoint": p.URL + "/authorize", "token_endpoint": p.URL + "/token", "jwks_uri": p.URL + "/jwks",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	case "/jwks":
		if broken {
			p.writeJSON(w, http.StatusOK, map[string]any{"keys": "broken"})
			return
		}
		p.writeJSON(w, http.StatusOK, map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.E)).Bytes()),
		}}})
	case "/authorize":
		if !autoLogin {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		code := base64.RawURLEncoding.EncodeToString(raw[:])
		p.RegisterCode(code, Claims{Subject: "e2e-subject", Email: "fixture@example.invalid", EmailVerified: true, Nonce: q.Get("nonce"), Challenge: q.Get("code_challenge"), RedirectURI: q.Get("redirect_uri")})
		redirect, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || redirect.Host == "" {
			http.Error(w, "invalid redirect", http.StatusBadRequest)
			return
		}
		values := redirect.Query()
		values.Set("code", code)
		values.Set("state", q.Get("state"))
		redirect.RawQuery = values.Encode()
		http.Redirect(w, r, redirect.String(), http.StatusFound)
	case "/token":
		p.serveToken(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Provider) serveToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		p.writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	p.mu.Lock()
	p.lastForm = r.PostForm
	c, ok := p.codes[r.PostForm.Get("code")]
	delete(p.codes, r.PostForm.Get("code"))
	p.mu.Unlock()
	if !ok || (c.Challenge != "" && oauth2.S256ChallengeFromVerifier(r.PostForm.Get("code_verifier")) != c.Challenge) ||
		(c.RedirectURI != "" && c.RedirectURI != r.PostForm.Get("redirect_uri")) || r.PostForm.Get("grant_type") != "authorization_code" {
		p.writeJSON(w, 400, map[string]string{"error": "invalid_grant", "error_description": "provider-private-detail"})
		return
	}
	response := map[string]any{"access_token": "test-access-token", "token_type": "Bearer", "expires_in": 3600}
	if !c.OmitIDToken {
		response["id_token"] = p.sign(c)
	}
	p.writeJSON(w, 200, response)
}

func (p *Provider) sign(c Claims) string {
	if c.Issuer == "" {
		c.Issuer = p.URL
	}
	if c.Audience == "" {
		c.Audience = ClientID
	}
	if c.ExpiresAt.IsZero() {
		c.ExpiresAt = time.Now().Add(time.Hour)
	}
	payload := map[string]any{"iss": c.Issuer, "aud": c.Audience, "sub": c.Subject, "email": c.Email, "nonce": c.Nonce, "exp": c.ExpiresAt.Unix(), "iat": time.Now().Unix()}
	if c.EmailVerified != nil {
		payload["email_verified"] = c.EmailVerified
	}
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key"})
	if err != nil {
		p.t.Error(err)
		return ""
	}
	body, err := json.Marshal(payload)
	if err != nil {
		p.t.Error(err)
		return ""
	}
	raw := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	sum := sha256.Sum256([]byte(raw))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		p.t.Error(err)
		return ""
	}
	if c.BadSignature {
		sig[0] ^= 1
	}
	return raw + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (p *Provider) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		p.t.Error(err)
	}
}
