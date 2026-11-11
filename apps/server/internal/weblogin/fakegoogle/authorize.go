package fakegoogle

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidcfake"
)

const formHTML = `<!doctype html><html><body><form method="post" action="/authorize">
{{range $name, $value := .Fields}}<input type="hidden" name="{{$name}}" value="{{$value}}">{{end}}
<label>Email <input name="email" type="text" value="{{.Email}}" required></label>
<label>Email verified <input name="email_verified" type="checkbox" checked></label>
<button type="submit">Continue</button></form></body></html>`

func (p *provider) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	q := r.Form
	if r.Method == http.MethodPost {
		q = r.PostForm
	}
	for _, key := range []string{"client_id", "redirect_uri", "response_type", "scope", "code_challenge", "code_challenge_method"} {
		if len(q[key]) != 1 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
	}
	scopes := strings.Fields(q.Get("scope"))
	openid := false
	for _, scope := range scopes {
		if scope == "openid" {
			openid = true
		}
	}
	if q.Get("client_id") != p.config.ClientID || q.Get("redirect_uri") != p.config.RedirectURI || q.Get("response_type") != "code" || !openid || q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	fields := map[string]string{}
	for _, key := range []string{"client_id", "redirect_uri", "response_type", "scope", "code_challenge", "code_challenge_method", "state", "nonce"} {
		fields[key] = q.Get(key)
	}
	email := q.Get("email")
	if r.Method == http.MethodGet || email == "" || len(email) > 254 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = p.form.Execute(w, struct {
			Fields map[string]string
			Email  string
		}{fields, email})
		return
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	code := base64.RawURLEncoding.EncodeToString(raw[:])
	hash := sha256.Sum256([]byte(email))
	p.core.RegisterCode(code, oidcfake.Claims{Subject: "fake-" + hex.EncodeToString(hash[:])[:20], Email: email, EmailVerified: q.Get("email_verified") != "", Nonce: q.Get("nonce"), Audience: p.config.ClientID, Challenge: q.Get("code_challenge"), RedirectURI: p.config.RedirectURI})
	target, err := url.Parse(p.config.RedirectURI)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	values := target.Query()
	values.Set("code", code)
	values.Set("state", q.Get("state"))
	target.RawQuery = values.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}
