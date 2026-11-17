package loadrun

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccounts_FakeGoogleFlow(t *testing.T) {
	var server *httptest.Server
	landings := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Error("missing account address")
		}
		switch r.URL.Path {
		case "/api/v1/auth/google/start":
			if r.URL.Query().Get("next") != "/account" {
				t.Error("missing next")
			}
			http.SetCookie(w, &http.Cookie{Name: "oauth", Value: "fixture", Path: "/", Secure: true})
			http.Redirect(w, r, server.URL+"/form", http.StatusFound)
		case "/form":
			_, _ = fmt.Fprint(w, `<form><input type="hidden" name="state" value="fixture-state"></form>`)
		case "/authorize":
			if r.Method != "POST" || r.FormValue("state") != "fixture-state" || !strings.HasSuffix(r.FormValue("email"), "@example.invalid") || r.FormValue("email_verified") != "on" {
				t.Error("invalid form")
			}
			http.Redirect(w, r, server.URL+"/api/v1/auth/google/callback", http.StatusFound)
		case "/api/v1/auth/google/callback":
			if _, err := r.Cookie("oauth"); err != nil {
				t.Error("missing OAuth cookie")
			}
			http.SetCookie(w, &http.Cookie{Name: "__Host-session", Value: "fixture-session", Path: "/", Secure: true})
			http.Redirect(w, r, "/account", http.StatusFound)
		case "/account":
			landings++
		case "/api/v1/me":
			if _, err := r.Cookie("__Host-session"); err != nil {
				t.Error("missing session")
			}
			_, _ = fmt.Fprint(w, `{"csrf_token":"fixture-csrf"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	got, err := accounts(context.Background(), Config{API: strings.Replace(server.URL, "127.0.0.1", "localhost", 1), Pages: server.URL, PublicURL: server.URL, Google: server.URL, Emails: []string{"one@example.invalid", "two@example.invalid", "three@example.invalid"}})
	if err != nil {
		t.Fatal(err)
	}
	if landings != 3 {
		t.Fatalf("landing visits = %d", landings)
	}
	if len(got) != 3 {
		t.Fatalf("accounts = %d", len(got))
	}
	for i, a := range got {
		if a.csrf != "fixture-csrf" || a.addr != address(i+100, 1000) {
			t.Fatal("invalid account")
		}
	}
}

func TestAccounts_RejectsMixedModes(t *testing.T) {
	_, err := accounts(context.Background(), Config{Sessions: []string{"a", "b", "c"}, Google: "http://127.0.0.1", Emails: []string{"one@example.invalid"}})
	if err == nil || err.Error() != "provide three sessions or three emails and fake Google" {
		t.Fatalf("unexpected error: %v", err)
	}
}
