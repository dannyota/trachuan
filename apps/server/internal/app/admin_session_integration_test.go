//go:build integration

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountapi"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func adminSessionDatabase(t *testing.T) (context.Context, *pgxpool.Pool, credentialStore) {
	t.Helper()
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	t.Setenv("SERVER_AUTH_DATABASE_URL", url)
	return ctx, pool, credentialStore{Queries: store.New(pool), db: pool}
}

func adminSessionAccount(t *testing.T, ctx context.Context, subject string) string {
	t.Helper()
	var out, stderr bytes.Buffer
	if err := run(ctx, []string{"admin", "account", "create", "--google-sub", subject, "--email", strings.ReplaceAll(subject, ":", "-") + "@example.test"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatal("account creation wrote diagnostics")
	}
	return strings.TrimSpace(out.String())
}

func TestAdminSession_RealStoreRefusesUnsafeCreation(t *testing.T) {
	ctx, pool, _ := adminSessionDatabase(t)
	for _, tc := range []struct {
		name, subject, hours, want string
		blocked, deleting, factor  bool
		existing, missing, unknown bool
	}{
		{name: "real", subject: "normal", want: "session requires a loadcheck account"},
		{name: "blocked", blocked: true, want: "account is blocked"},
		{name: "deleting", deleting: true, want: "account is being deleted"},
		{name: "existing", existing: true, want: "create session file failed"},
		{name: "parent", missing: true, want: "create session file failed"},
		{name: "zero", hours: "0", want: "invalid session fields"},
		{name: "large", hours: "25", want: "invalid session fields"},
		{name: "factor", factor: true, want: "create session failed"},
		{name: "unknown", unknown: true, want: "account not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subject := tc.subject
			if subject == "" {
				subject = "loadcheck:" + tc.name
			}
			id := adminSessionAccount(t, ctx, subject)
			accountID, parseErr := strconv.ParseInt(id, 10, 64)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if tc.blocked {
				if _, err := pool.Exec(ctx, `UPDATE account SET blocked_at=now() WHERE id=$1`, accountID); err != nil {
					t.Fatal(err)
				}
			}
			if tc.deleting {
				if _, err := pool.Exec(ctx, `UPDATE account SET delete_requested_at=now() WHERE id=$1`, accountID); err != nil {
					t.Fatal(err)
				}
			}
			if tc.factor {
				if _, err := pool.Exec(ctx, `INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES ($1,$2,'synthetic')`, accountID, []byte("synthetic sealed secret")); err != nil {
					t.Fatal(err)
				}
			}
			if tc.unknown {
				id = "9223372036854775807"
			}
			path := filepath.Join(t.TempDir(), "session")
			if tc.existing {
				if err := os.WriteFile(path, []byte("keep\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.missing {
				path = filepath.Join(path, "session")
			}
			args := []string{"admin", "session", "create", "--account", id, "--out", path}
			if tc.hours != "" {
				args = append(args, "--hours", tc.hours)
			}
			var out, stderr bytes.Buffer
			err := run(ctx, args, &out, &stderr)
			if err == nil || err.Error() != tc.want || out.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected refusal: %v", err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM web_session`).Scan(&count); err != nil || count != 0 {
				t.Fatal("refusal wrote a session")
			}
			if tc.existing {
				data, readErr := os.ReadFile(path)
				if readErr != nil || string(data) != "keep\n" {
					t.Fatal("existing file changed")
				}
			} else if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatal("refusal left a file")
			}
		})
	}
}

func TestAdminSession_AuthenticatesExpiresAndRevokes(t *testing.T) {
	ctx, pool, q := adminSessionDatabase(t)
	for _, hours := range []string{"", "1", "24"} {
		t.Run("hours-"+hours, func(t *testing.T) {
			id := adminSessionAccount(t, ctx, "loadcheck:success"+hours)
			if _, epochErr := pool.Exec(ctx, `UPDATE account SET auth_epoch=7 WHERE google_sub=$1`, "loadcheck:success"+hours); epochErr != nil {
				t.Fatal(epochErr)
			}
			path := filepath.Join(t.TempDir(), "session")
			args := []string{"admin", "session", "create", "--account", id, "--out", path}
			n := 24
			if hours != "" {
				args = append(args, "--hours", hours)
				n, _ = strconv.Atoi(hours)
			}
			var out, stderr bytes.Buffer
			started := time.Now()
			if code := exitCode(run(ctx, args, &out, &stderr), &stderr); code != 0 {
				t.Fatalf("command failed with code %d", code)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 || len(data) != 44 || data[43] != '\n' {
				t.Fatal("session file has wrong mode or shape")
			}
			raw := string(data[:43])
			if decoded, decodeErr := base64.RawURLEncoding.Strict().DecodeString(raw); decodeErr != nil || len(decoded) != 32 {
				t.Fatal("session value is not 32 random bytes")
			}
			if out.Len() != 0 || stderr.Len() != 0 {
				t.Fatal("command exposed output")
			}
			digest := sha256.Sum256([]byte(raw))
			hash := hex.EncodeToString(digest[:])
			row, err := q.GetSession(ctx, hash)
			lifetime := time.Duration(n) * time.Hour
			if err != nil || row.ExpiresAt.After(row.CreatedAt.Add(lifetime)) || row.ExpiresAt.Before(started.Add(lifetime-time.Second)) || len(row.CsrfSecret) != 43 || row.ProofAt == nil || row.SessionAuthEpoch != 7 || row.SessionAuthEpoch != row.AccountAuthEpoch {
				t.Fatal("session metadata differs from login or lifetime exceeds limit")
			}
			clock := time.Now
			sessions := &websession.Manager{Store: q, Clock: func() time.Time { return clock() }}
			verifier := &credential.Verifier{Store: q}
			handler := sessions.Middleware(accountapi.New(accountapi.Config{Store: q, Sessions: sessions, Verifier: verifier, Limiter: limits.New(time.Now)}))
			me := func() *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
				request.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				return response
			}
			response := me()
			var identity struct {
				ID   int64  `json:"id"`
				CSRF string `json:"csrf_token"`
			}
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &identity) != nil || strconv.FormatInt(identity.ID, 10) != id || identity.CSRF != row.CsrfSecret {
				t.Fatal("session did not authenticate /api/v1/me")
			}
			if _, ageErr := pool.Exec(ctx, `UPDATE web_session SET last_seen_at=now()-interval '2 hours' WHERE id=$1`, row.ID); ageErr != nil {
				t.Fatal(ageErr)
			}
			clock = func() time.Time { return time.Now().Add(11 * time.Second) }
			response = me()
			updated, err := q.GetSession(ctx, hash)
			if response.Code != http.StatusOK || err != nil || !updated.ExpiresAt.Equal(row.ExpiresAt) || !updated.LastSeenAt.After(time.Now().Add(-time.Minute)) {
				t.Fatal("activity changed the absolute expiry")
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 1 || cookies[0].MaxAge > n*3600 {
				t.Fatal("renewal exceeded absolute expiry")
			}
			clock = func() time.Time { return row.ExpiresAt.Add(time.Second) }
			if me().Code != http.StatusNoContent {
				t.Fatal("session authenticated after absolute expiry")
			}
			clock = time.Now
			if me().Code != http.StatusOK {
				t.Fatal("fixture did not restore time")
			}
			if blockErr := run(ctx, []string{"admin", "account", "block", "--account", id}, &out, &stderr); blockErr != nil {
				t.Fatal(blockErr)
			}
			clock = func() time.Time { return time.Now().Add(11 * time.Second) }
			if me().Code != http.StatusNoContent {
				t.Fatal("blocked account session still authenticates")
			}
			updated, err = q.GetSession(ctx, hash)
			if err != nil || updated.RevokedAt == nil || updated.AccountAuthEpoch <= updated.SessionAuthEpoch {
				t.Fatal("blocking did not revoke the session")
			}
		})
	}
}
