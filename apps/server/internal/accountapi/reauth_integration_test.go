//go:build integration

package accountapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTokens_PostgresReauth(t *testing.T) {
	for _, tc := range []struct {
		name, proof, factor string
		enrolled            bool
		want                int
		factorBody          bool
	}{
		{"legacy", "NULL", "NULL", false, 403, false},
		{"stale proof", "now()-interval '11 minutes'", "NULL", false, 403, false},
		{"recent", "now()", "NULL", false, 201, false},
		{"factor missing", "now()", "NULL", true, 403, true},
		{"factor stale", "now()", "now()-interval '11 minutes'", true, 403, true},
		{"factor recent", "now()", "now()", true, 201, false},
		{"primary first", "NULL", "now()", true, 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := testpg.NewDatabase(t)
			if err := migrate.Up(t.Context(), dsn); err != nil {
				t.Fatal(err)
			}
			pool, err := pgxpool.New(t.Context(), dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pool.Close)
			q := store.New(pool)
			a, err := q.CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: "proof", Email: "proof@example.test"})
			if err != nil {
				t.Fatal(err)
			}
			m := &websession.Manager{Store: q, PublicURL: "https://example.invalid"}
			w := httptest.NewRecorder()
			if err = m.CreateSession(t.Context(), w, httptest.NewRequest("GET", "/", nil), a.ID); err != nil {
				t.Fatal(err)
			}
			raw := w.Result().Cookies()[0].Value
			session, err := q.GetSession(t.Context(), hash(raw))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(t.Context(), `UPDATE web_session SET proof_at=`+tc.proof+`,factor_at=`+tc.factor+` WHERE id=$1`, session.ID); err != nil {
				t.Fatal(err)
			}
			if tc.enrolled {
				if _, err := pool.Exec(t.Context(), `INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES ($1,'synthetic','synthetic')`, a.ID); err != nil {
					t.Fatal(err)
				}
			}
			v := &credential.Verifier{Store: q, Audience: "mcp"}
			h := m.Middleware(m.CSRF(New(Config{Store: q, Sessions: m, Verifier: v, Limiter: limits.New(time.Now)})))
			w = call(h, raw, session.CsrfSecret, "POST", "/api/v1/me/tokens", `{"name":"synthetic"}`)
			expect(t, w, tc.want)
			if tc.want == 403 {
				var body struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
					Factor bool `json:"factor"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Error.Code != "reauth_required" || body.Factor != tc.factorBody {
					t.Fatal(w.Body.String())
				}
			}
			var count int
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM personal_token`).Scan(&count); err != nil || (count == 1) != (tc.want == 201) {
				t.Fatal("incorrect token count")
			}
			if tc.want == 403 && strings.Contains(w.Body.String(), "trc_") {
				t.Fatal("refusal leaked token")
			}
		})
	}
}
