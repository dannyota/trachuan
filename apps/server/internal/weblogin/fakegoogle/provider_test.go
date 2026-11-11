package fakegoogle

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func fixture(t *testing.T) (http.Handler, url.Values) {
	t.Helper()
	p, err := New(Config{Issuer: "http://127.0.0.1:28081", ClientID: "client", ClientSecret: "secret", RedirectURI: "http://127.0.0.1:28080/callback"})
	if err != nil {
		t.Fatal(err)
	}
	return p, url.Values{"client_id": {"client"}, "redirect_uri": {"http://127.0.0.1:28080/callback"}, "response_type": {"code"}, "scope": {"openid email"}, "code_challenge": {"challenge"}, "code_challenge_method": {"S256"}, "state": {"state"}, "nonce": {"nonce"}}
}
func request(h http.Handler, method, path string, values url.Values) *httptest.ResponseRecorder {
	var r *http.Request
	if method == http.MethodGet {
		r = httptest.NewRequest(method, path+"?"+values.Encode(), nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestAuthorization_RejectsInvalidRequests(t *testing.T) {
	for _, tc := range []struct{ key, value string }{{"client_id", "other"}, {"redirect_uri", "http://evil.invalid"}, {"response_type", "token"}, {"scope", "email"}, {"code_challenge", ""}, {"code_challenge_method", "plain"}} {
		t.Run(tc.key, func(t *testing.T) {
			h, q := fixture(t)
			q.Set(tc.key, tc.value)
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				w := request(h, method, "/authorize", q)
				if w.Code != 400 || w.Header().Get("Location") != "" {
					t.Fatalf("%s: status %d, redirect %q", method, w.Code, w.Header().Get("Location"))
				}
			}
		})
	}
}
func TestAuthorization_EscapesEmail(t *testing.T) {
	h, q := fixture(t)
	// An overlong email is returned on a validated request for correction.
	q.Set("code_challenge", "challenge")
	q.Set("email", "<script>"+strings.Repeat("x", 254))
	w := request(h, http.MethodPost, "/authorize", q)
	if w.Code != 400 || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatalf("status %d, email not safely rendered", w.Code)
	}
}
func TestToken_SingleUseAndBinding(t *testing.T) {
	for _, tc := range []struct {
		name, verifier, redirect string
		want                     int
	}{{"valid", strings.Repeat("a", 43), "http://127.0.0.1:28080/callback", 200}, {"verifier", strings.Repeat("b", 43), "http://127.0.0.1:28080/callback", 400}, {"redirect", strings.Repeat("a", 43), "http://127.0.0.1:28080/other", 400}} {
		t.Run(tc.name, func(t *testing.T) {
			h, q := fixture(t)
			q.Set("code_challenge", oauth2.S256ChallengeFromVerifier(strings.Repeat("a", 43)))
			q.Set("email", "a@example.invalid")
			q.Set("email_verified", "on")
			w := request(h, http.MethodPost, "/authorize", q)
			if w.Code != 302 {
				t.Fatalf("authorization status %d", w.Code)
			}
			u, err := url.Parse(w.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			v := url.Values{"grant_type": {"authorization_code"}, "code": {u.Query().Get("code")}, "code_verifier": {tc.verifier}, "redirect_uri": {tc.redirect}, "client_id": {"client"}, "client_secret": {"secret"}}
			w = request(h, http.MethodPost, "/token", v)
			if w.Code != tc.want {
				t.Fatalf("token status %d", w.Code)
			}
			if tc.want == 200 {
				w = request(h, http.MethodPost, "/token", v)
				if w.Code != 400 {
					t.Fatalf("reused code status %d", w.Code)
				}
			}
		})
	}
}
func TestListen_RejectsNonLoopback(t *testing.T) {
	for _, v := range []string{":8080", "0.0.0.0:8080", "localhost:0", "localhost:+80", "localhost", "LOCALHOST:8080", "[::ffff:127.0.0.1]:8080"} {
		if ValidListen(v) {
			t.Errorf("accepted %q", v)
		}
	}
	for _, v := range []string{"127.0.0.1:28081", "localhost:28081", "[::1]:28081"} {
		if !ValidListen(v) {
			t.Errorf("rejected %q", v)
		}
	}
}
func TestToken_RequiresClientAuthentication(t *testing.T) {
	h, q := fixture(t)
	verifier := strings.Repeat("a", 43)
	q.Set("code_challenge", oauth2.S256ChallengeFromVerifier(verifier))
	q.Set("email", "auth@example.invalid")
	w := request(h, http.MethodPost, "/authorize", q)
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil || w.Code != 302 {
		t.Fatal("code not issued")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {u.Query().Get("code")}, "code_verifier": {verifier}, "redirect_uri": {"http://127.0.0.1:28080/callback"}, "client_id": {"client"}, "client_secret": {"wrong"}}
	w = request(h, http.MethodPost, "/token", form)
	if w.Code != 400 {
		t.Fatalf("invalid secret status %d", w.Code)
	}
	form.Set("client_secret", "secret")
	w = request(h, http.MethodPost, "/token", form)
	if w.Code != 200 {
		t.Fatalf("valid secret status %d", w.Code)
	}
}

func TestAuthorization_FormDefaults(t *testing.T) {
	h, q := fixture(t)
	w := request(h, http.MethodGet, "/authorize", q)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `name="email"`) || !strings.Contains(body, `name="email_verified" type="checkbox" checked`) || !strings.Contains(body, `name="state" value="state"`) {
		t.Fatal("missing form fields or default")
	}
}

func TestToken_BasicAuthenticationAndIdentity(t *testing.T) {
	h, q := fixture(t)
	verifier := strings.Repeat("a", 43)
	q.Set("code_challenge", oauth2.S256ChallengeFromVerifier(verifier))
	q.Set("email", "identity@example.invalid")
	q.Set("email_verified", "on")
	w := request(h, http.MethodPost, "/authorize", q)
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil || w.Code != 302 {
		t.Fatal("code not issued")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {u.Query().Get("code")}, "code_verifier": {verifier}, "redirect_uri": {"http://127.0.0.1:28080/callback"}}
	r := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetBasicAuth("client", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("token status %d", w.Code)
	}
	var response struct {
		IDToken string `json:"id_token"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(response.IDToken, ".")
	if len(parts) != 3 {
		t.Fatal("invalid token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Subject  string `json:"sub"`
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Audience string `json:"aud"`
		Nonce    string `json:"nonce"`
	}
	if err = json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "fake-de1689bd0dd0ac2fe7b8" || claims.Email != "identity@example.invalid" || !claims.Verified || claims.Audience != "client" || claims.Nonce != "nonce" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestAuthorization_EmailLength(t *testing.T) {
	for _, tc := range []struct {
		name, email string
		want        int
	}{
		{"empty", "", 400},
		{"maximum", strings.Repeat("x", 254), 302},
		{"too long", strings.Repeat("x", 255), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, q := fixture(t)
			q.Set("email", tc.email)
			w := request(h, http.MethodPost, "/authorize", q)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
		})
	}
}
