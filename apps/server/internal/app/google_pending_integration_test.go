//go:build integration

package app

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
)

func TestGoogle_ConsumesPendingBoundAtStartWithoutCallbackCookie(t *testing.T) {
	for _, scenario := range []string{"bound session", "bound factor", "absent", "forged", "malformed", "duplicate", "failed callback"} {
		t.Run(scenario, func(t *testing.T) {
			h, db, p := signupConfigHandler(t, nil)
			var id int64
			if err := db.QueryRow(t.Context(), `INSERT INTO account(google_sub,email) VALUES('prior','prior@example.test') RETURNING id`).Scan(&id); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(t.Context(), `INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES($1,'synthetic','synthetic')`, id); err != nil {
				t.Fatal(err)
			}
			raw := strings.Repeat("A", 43)
			sum := sha256.Sum256([]byte(raw))
			hash := hex.EncodeToString(sum[:])
			otherRaw := strings.Repeat("B", 43)
			sum = sha256.Sum256([]byte(otherRaw))
			other := hex.EncodeToString(sum[:])
			if _, err := db.Exec(t.Context(), `INSERT INTO pending_auth(token_hash,csrf_secret,account_id,auth_epoch,primary_kind,primary_at,expires_at) SELECT token,'synthetic',$1,0,'google',now(),now()+interval '5 minutes' FROM unnest($2::text[]) token`, id, []string{hash, other}); err != nil {
				t.Fatal(err)
			}
			if scenario == "bound factor" {
				if _, err := db.Exec(t.Context(), `INSERT INTO account(google_sub,email) VALUES('target','target@example.test'); INSERT INTO totp_credential(account_id,sealed_secret,key_id) SELECT id,'synthetic','synthetic' FROM account WHERE google_sub='target'`); err != nil {
					t.Fatal(err)
				}
			}
			start := httptest.NewRequest("GET", "/api/v1/auth/google/start?next=/destination&pending_hash="+other, nil)
			switch scenario {
			case "bound session", "bound factor", "failed callback":
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: raw})
			case "malformed":
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: "malformed"})
			case "duplicate":
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: raw})
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: otherRaw})
			}
			w := httptest.NewRecorder()
			h.Start.ServeHTTP(w, start)
			if w.Code != 302 {
				t.Fatal("Google start failed")
			}
			location := w.Header().Get("Location")
			if strings.Contains(location, raw) || strings.Contains(location, hash) {
				t.Fatal("binding exposed to Google")
			}
			u, err := url.Parse(location)
			if err != nil {
				t.Fatal(err)
			}
			q := u.Query()
			p.RegisterCode("synthetic-code", oidctest.Claims{Subject: "target", Email: "target@example.test", EmailVerified: true, Nonce: q.Get("nonce"), Challenge: q.Get("code_challenge"), RedirectURI: q.Get("redirect_uri")})
			state := q.Get("state")
			if scenario == "failed callback" {
				state = "wrong"
			}
			callback := httptest.NewRequest("GET", "/api/v1/auth/google/callback?"+url.Values{"state": {state}, "code": {"synthetic-code"}, "pending_hash": {other}}.Encode(), nil)
			for _, cookie := range w.Result().Cookies() {
				if cookie.Name == "__Host-login" {
					callback.AddCookie(cookie)
				}
			}
			if scenario == "forged" {
				callback.AddCookie(&http.Cookie{Name: "__Host-pending", Value: otherRaw})
			}
			w = httptest.NewRecorder()
			h.Callback.ServeHTTP(w, callback)
			want := "/destination"
			if scenario == "bound factor" {
				want = "/login/factor"
			}
			if scenario == "failed callback" {
				want = "/login?error=login_failed"
			}
			if w.Code != 302 || w.Header().Get("Location") != want {
				t.Fatal("callback failed", w.Code)
			}
			var consumed, otherConsumed bool
			if err = db.QueryRow(t.Context(), `SELECT (SELECT consumed_at IS NOT NULL FROM pending_auth WHERE token_hash=$1),(SELECT consumed_at IS NOT NULL FROM pending_auth WHERE token_hash=$2)`, hash, other).Scan(&consumed, &otherConsumed); err != nil {
				t.Fatal(err)
			}
			bound := scenario == "bound session" || scenario == "bound factor"
			if consumed != bound || otherConsumed {
				t.Fatal("callback consumed an unbound pending authentication")
			}
			if bound {
				found := false
				for _, cookie := range w.Result().Cookies() {
					if cookie.Name == "__Host-pending" {
						if scenario == "bound session" {
							found = cookie.MaxAge < 0
						} else {
							found = cookie.MaxAge == 300 && cookie.Value != raw
						}
					}
				}
				if !found {
					t.Fatal("prior pending cookie not cleared or replaced")
				}
			}
		})
	}
}
