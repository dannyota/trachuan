//go:build integration

package app

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/seal"
)

func TestPassword_EnrolledPendingCompletionAndExpiry(t *testing.T) {
	f := newE2E(t)
	hash, err := password.NewHasher().Hash(t.Context(), "synthetic passphrase")
	if err != nil {
		t.Fatal(err)
	}
	id := passwordAccount(t, f, "factor@example.test", hash)
	var rowID int64
	if err = f.pool.QueryRow(t.Context(), `INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES($1,''::bytea,'') RETURNING id`, id).Scan(&rowID); err != nil {
		t.Fatal(err)
	}
	ring, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(make([]byte, 32))))
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("12345678901234567890")
	data, key, err := ring.Seal(seal.Binding{Purpose: "totp", OwnerKind: "account", OwnerID: id, RowID: rowID}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(t.Context(), `UPDATE totp_credential SET sealed_secret=$2,key_id=$3 WHERE id=$1`, rowID, data, key); err != nil {
		t.Fatal(err)
	}
	client := f.client()
	signIn := func() string {
		response := f.postWithHeaders(client, "/api/v1/auth/password", `{"email":"factor@example.test","password":"synthetic passphrase","next":"/oauth/consent/synthetic"}`, "application/json", f.server.URL, "")
		pending := false
		for _, cookie := range response.Cookies() {
			if cookie.Name == "__Host-session" && cookie.MaxAge >= 0 {
				t.Fatal("primary issued session")
			}
			if cookie.Name == "__Host-pending" && cookie.MaxAge == 300 && cookie.SameSite == http.SameSiteStrictMode && cookie.HttpOnly && cookie.Secure && cookie.Path == "/" {
				pending = true
			}
		}
		var body struct {
			Required bool   `json:"factor_required"`
			CSRF     string `json:"csrf"`
		}
		f.json(response, 200, &body)
		if !body.Required || len(body.CSRF) != 43 || !pending {
			t.Fatal("missing pending authentication")
		}
		return body.CSRF
	}
	csrf := signIn()
	f.status(f.request(client, "GET", "/api/v1/me", "", "", "", ""), 204)
	var pending struct{ CSRF, Next string }
	f.json(f.request(client, "GET", "/api/v1/auth/factor", "", "", "", ""), 200, &pending)
	if pending.CSRF != csrf || pending.Next != "/oauth/consent/synthetic" {
		t.Fatal("pending bootstrap lost csrf or destination")
	}
	f.status(f.postWithHeaders(client, "/api/v1/auth/factor", `{"code":"wrong"}`, "application/json", f.server.URL, "wrong"), 403)
	response := f.postWithHeaders(client, "/api/v1/auth/factor", `{"code":"`+factor.Code(secret, time.Now().Unix()/30)+`"}`, "application/json", f.server.URL, csrf)
	cleared := false
	for _, cookie := range response.Cookies() {
		if cookie.Name == "__Host-pending" && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	var destination struct{ Next string }
	f.json(response, 200, &destination)
	if !cleared || destination.Next != "/oauth/consent/synthetic" || f.me(client).ID != id {
		t.Fatal("factor completion lost session or destination")
	}
	var proofs bool
	if err = f.pool.QueryRow(t.Context(), `SELECT s.factor_at IS NOT NULL AND s.proof_at=p.primary_at FROM web_session s JOIN pending_auth p ON p.account_id=s.account_id WHERE s.account_id=$1 AND s.revoked_at IS NULL AND p.consumed_at IS NOT NULL`, id).Scan(&proofs); err != nil || !proofs {
		t.Fatal("completion proof times", err)
	}
	signIn()
	if _, err = f.pool.Exec(t.Context(), `UPDATE pending_auth SET expires_at=now()-interval '1 second' WHERE account_id=$1 AND consumed_at IS NULL`, id); err != nil {
		t.Fatal(err)
	}
	response = f.request(client, "GET", "/api/v1/auth/factor", "", "", "", "")
	cleared = false
	for _, cookie := range response.Cookies() {
		if cookie.Name == "__Host-pending" && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	f.status(response, 401)
	if !cleared {
		t.Fatal("expired pending cookie survived")
	}
}
