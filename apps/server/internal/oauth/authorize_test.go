package oauth

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestAuthorize_ValidatesAll(t *testing.T) {
	for _, tc := range []struct{ key, value, code string }{
		{"client_id", "unknown", "invalid_client"}, {"redirect_uri", "https://attacker.example/cb", "invalid_request"}, {"response_type", "token", "unsupported_response_type"}, {"code_challenge", "bad", "invalid_request"}, {"code_challenge_method", "plain", "invalid_request"}, {"state", strings.Repeat("x", 513), "invalid_request"}, {"resource", "https://other.example/mcp", "invalid_target"}, {"resource", "", "invalid_target"}, {"scope", "mcp:write", "invalid_scope"},
	} {
		t.Run(tc.key+tc.value[:min(len(tc.value), 20)], func(t *testing.T) {
			d := newConsentDB()
			v := authorizeValues()
			v.Set(tc.key, tc.value)
			w := authorizeFor(t, d, v)
			if w.Code != 400 || !strings.Contains(w.Body.String(), tc.code) || d.pending.HandleHash != "" {
				t.Fatalf("status %d body %s", w.Code, w.Body)
			}
		})
	}
	for _, key := range []string{"client_id", "redirect_uri", "response_type", "code_challenge", "code_challenge_method", "state", "resource", "scope"} {
		t.Run("duplicate "+key, func(t *testing.T) {
			v := authorizeValues()
			if len(v[key]) == 0 {
				v.Set(key, "")
			}
			v.Add(key, v.Get(key))
			w := authorizeFor(t, newConsentDB(), v)
			if w.Code != 400 {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	if len(path) >= 512 || d.pending.Scope != "mcp:read" || d.pending.Resource != consentBase+"/mcp" || d.pending.HandleHash != consentHash(strings.TrimPrefix(path, "/oauth/consent/")) || d.pending.BrowserHash != consentHash(cookie.Value) {
		t.Fatal("pending bindings")
	}
	if cookie.Name != "__Host-oauth" || cookie.Path != "/" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != 2 || cookie.MaxAge != 600 {
		t.Fatalf("cookie attributes: %+v", cookie)
	}
}
func TestAuthorize_ErrorsStayOnServerPage(t *testing.T) {
	v := authorizeValues()
	v.Set("scope", "bad")
	w := authorizeFor(t, newConsentDB(), v)
	if w.Header().Get("Location") != "" || w.Code != 400 {
		t.Fatal("automatic redirect")
	}
	u := submittedErrorForm(t, w.Body.String())
	if u.Host != "localhost" || u.Query().Get("error") != "invalid_scope" || u.Query().Get("state") != v.Get("state") || u.Query().Get("iss") != consentBase {
		t.Fatalf("return action %q", u)
	}
	v.Set("redirect_uri", "https://attacker.example/cb")
	w = authorizeFor(t, newConsentDB(), v)
	if strings.Contains(w.Body.String(), "<form") {
		t.Fatal("untrusted return button")
	}
}
func TestAuthorize_PendingCap503(t *testing.T) {
	d := newConsentDB()
	d.pendingCount = 10000
	w := authorizeFor(t, d, authorizeValues())
	if w.Code != 503 || !strings.Contains(w.Body.String(), "temporarily_unavailable") || d.pending.HandleHash != "" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("status %d", w.Code)
	}
}

func submittedErrorForm(t *testing.T, body string) *url.URL {
	t.Helper()
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var action string
	fields := url.Values{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if n.Data == "form" {
				action = attrs["action"]
			}
			if n.Data == "input" && attrs["type"] == "hidden" {
				fields.Add(attrs["name"], attrs["value"])
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if action == "" {
		t.Fatal("missing return button")
	}
	u, err := url.Parse(action)
	if err != nil {
		t.Fatal(err)
	}
	u.RawQuery = fields.Encode()
	return u
}

func TestAuthorize_ErrorStateIsBounded(t *testing.T) {
	v := authorizeValues()
	v.Set("state", strings.Repeat("x", 513))
	v.Add("scope", "bad")
	v.Add("scope", "bad")
	w := authorizeFor(t, newConsentDB(), v)
	u := submittedErrorForm(t, w.Body.String())
	if len(u.Query().Get("state")) > 512 {
		t.Fatal("oversize state reflected in return button")
	}
}
