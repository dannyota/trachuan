package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const tokenTestBase = "https://example.invalid"

func tokenDigest(raw string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) }
func tokenVerifier() string         { return strings.Repeat("v", 43) }
func tokenChallenge() string {
	sum := sha256.Sum256([]byte(tokenVerifier()))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func codeForm() url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "client_id": {"client"}, "resource": {tokenTestBase + "/mcp"}, "code": {"fixture-code"}, "code_verifier": {tokenVerifier()}}
}
func refreshForm(raw string) url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "client_id": {"client"}, "resource": {tokenTestBase + "/mcp"}, "refresh_token": {raw}}
}
func requestToken(h http.Handler, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func tokenJSON(t *testing.T, w *httptest.ResponseRecorder, status int, code string) map[string]any {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d", w.Code, status)
	}
	for key, want := range map[string]string{"Content-Type": "application/json", "Cache-Control": "no-store", "Pragma": "no-cache"} {
		if w.Header().Get(key) != want {
			t.Errorf("%s = %q", key, w.Header().Get(key))
		}
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if code != "" && got["error"] != code {
		t.Fatalf("error = %v, want %s", got["error"], code)
	}
	if code != "" && (got["access_token"] != nil || got["refresh_token"] != nil) {
		t.Fatal("error exposed tokens")
	}
	return got
}

type tokenMemory struct {
	mu                  sync.Mutex
	account             store.Account
	code                store.ConsumeCodeRow
	tokens              map[string]store.GetTokenRow
	grants              []string
	revoked             bool
	beginErr, commitErr error
	fail                string
	invalidations       int
	commits, rollbacks  int
}

func newTokenMemory() *tokenMemory {
	now := time.Now()
	return &tokenMemory{account: store.Account{ID: 1, AuthEpoch: 7}, code: store.ConsumeCodeRow{CodeHash: tokenDigest("fixture-code"), GrantID: 2, AccountID: 1, ClientID: "client", RedirectUri: "http://localhost/cb", Resource: tokenTestBase + "/mcp", CodeChallenge: tokenChallenge(), Scope: "mcp:read", AuthEpoch: 7, ExpiresAt: now.Add(time.Minute)}, tokens: map[string]store.GetTokenRow{}, grants: []string{"authorization_code", "refresh_token"}}
}
func (m *tokenMemory) handler() http.Handler {
	return NewTokenHandler(m, tokenTestBase, func(int64) { m.invalidations++ })
}
func (m *tokenMemory) Begin(context.Context) (pgx.Tx, error) {
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	m.mu.Lock()
	tokens := map[string]store.GetTokenRow{}
	for k, v := range m.tokens {
		tokens[k] = v
	}
	return &tokenMemoryTx{m: m, code: m.code, tokens: tokens, revoked: m.revoked}, nil
}

type tokenMemoryTx struct {
	pgx.Tx
	m               *tokenMemory
	code            store.ConsumeCodeRow
	tokens          map[string]store.GetTokenRow
	revoked, closed bool
}

func (tx *tokenMemoryTx) Commit(context.Context) error {
	tx.m.code = tx.code
	tx.m.tokens = tx.tokens
	tx.m.revoked = tx.revoked
	tx.m.commits++
	tx.closed = true
	tx.m.mu.Unlock()
	return tx.m.commitErr
}
func (tx *tokenMemoryTx) Rollback(context.Context) error {
	if !tx.closed {
		tx.closed = true
		tx.m.rollbacks++
		tx.m.mu.Unlock()
	}
	return nil
}
func (tx *tokenMemoryTx) check(ctx context.Context, sql string) error {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 2*time.Second {
		return errors.New("query lacks 2 second deadline")
	}
	if tx.m.fail != "" && strings.Contains(sql, tx.m.fail) {
		return errors.New("database unavailable")
	}
	return nil
}
func (tx *tokenMemoryTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if err := tx.check(ctx, sql); err != nil {
		return pgconn.CommandTag{}, err
	}
	if strings.Contains(sql, "name: PruneTokenFamilies") {
		revoked := 0
		families := map[string]time.Time{}
		for _, v := range tx.tokens {
			if v.RevokedAt == nil && v.FamilyExpiresAt.After(time.Now()) {
				if at, ok := families[v.FamilyID]; !ok || v.CreatedAt.Before(at) {
					families[v.FamilyID] = v.CreatedAt
				}
			}
		}
		for len(families) > 9 {
			oldest := ""
			var at time.Time
			for id, created := range families {
				if oldest == "" || created.Before(at) {
					oldest = id
					at = created
				}
			}
			now := time.Now()
			for hash, v := range tx.tokens {
				if v.FamilyID == oldest {
					v.RevokedAt = &now
					tx.tokens[hash] = v
					revoked++
				}
			}
			delete(families, oldest)
		}
		return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", revoked)), nil
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type tokenScan struct {
	values []any
	err    error
}

func (s tokenScan) Scan(dst ...any) error {
	if s.err != nil {
		return s.err
	}
	if len(dst) != len(s.values) {
		return fmt.Errorf("scan fields %d, want %d", len(dst), len(s.values))
	}
	for i, v := range s.values {
		reflect.ValueOf(dst[i]).Elem().Set(reflect.ValueOf(v))
	}
	return nil
}
func tokenStructRow(v any) pgx.Row {
	value := reflect.ValueOf(v)
	fields := make([]any, value.NumField())
	for i := range fields {
		fields[i] = value.Field(i).Interface()
	}
	return tokenScan{values: fields}
}
func (tx *tokenMemoryTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if err := tx.check(ctx, sql); err != nil {
		return tokenScan{err: err}
	}
	switch {
	case strings.Contains(sql, "name: GetClient"):
		if args[0] != "client" && args[0] != "other" {
			return tokenScan{err: pgx.ErrNoRows}
		}
		return tokenStructRow(store.OauthClient{ClientID: args[0].(string), GrantTypes: tx.m.grants})
	case strings.Contains(sql, "name: TouchClient"):
		return tokenStructRow(store.OauthClient{ClientID: args[0].(string)})
	case strings.Contains(sql, "name: GetCodeIdentity"):
		if args[0] != tx.code.CodeHash {
			return tokenScan{err: pgx.ErrNoRows}
		}
		return tokenScan{values: []any{tx.code.AccountID, tx.code.GrantID}}
	case strings.Contains(sql, "name: LockCredentialAccount"):
		return tokenStructRow(tx.m.account)
	case strings.Contains(sql, "name: LockGrantAuthority"):
		var revoked *time.Time
		if tx.revoked {
			now := time.Now()
			revoked = &now
		}
		return tokenScan{values: []any{int64(1), "client", tokenTestBase + "/mcp", "mcp:read", revoked}}
	case strings.Contains(sql, "name: ConsumeCode"):
		if args[0] != tx.code.CodeHash || tx.code.RevokedAt != nil || (tx.code.ConsumedAt == nil && !tx.code.ExpiresAt.After(time.Now())) {
			return tokenScan{err: pgx.ErrNoRows}
		}
		replay := tx.code.ConsumedAt != nil
		now := time.Now()
		tx.code.ConsumedAt = &now
		row := tx.code
		row.Replay = replay
		return tokenStructRow(row)
	case strings.Contains(sql, "name: RevokeGrant"):
		tx.revoked = true
		now := time.Now()
		tx.code.RevokedAt = &now
		for k, v := range tx.tokens {
			v.RevokedAt = &now
			tx.tokens[k] = v
		}
		return tokenStructRow(store.RevokeGrantRow{ID: 2, RevokedAt: &now})
	case strings.Contains(sql, "name: GetToken"):
		v, ok := tx.tokens[args[0].(string)]
		if !ok {
			return tokenScan{err: pgx.ErrNoRows}
		}
		return tokenStructRow(v)
	case strings.Contains(sql, "name: RotateRefresh"):
		v, ok := tx.tokens[args[0].(string)]
		if !ok {
			return tokenScan{err: pgx.ErrNoRows}
		}
		reuse := v.SupersededAt != nil
		now := time.Now()
		v.SupersededAt = &now
		tx.tokens[v.TokenHash] = v
		return tokenStructRow(store.RotateRefreshRow{TokenHash: v.TokenHash, Kind: v.Kind, GrantID: v.GrantID, AccountID: v.AccountID, ClientID: v.ClientID, FamilyID: v.FamilyID, Resource: v.Resource, Scope: v.Scope, AuthEpoch: v.AuthEpoch, ExpiresAt: v.ExpiresAt, FamilyExpiresAt: v.FamilyExpiresAt, RevokedAt: v.RevokedAt, SupersededAt: v.SupersededAt, Reuse: reuse})
	case strings.Contains(sql, "name: CreateTokenFamily"):
		v := store.GetTokenRow{TokenHash: args[0].(string), Kind: args[1].(string), GrantID: args[9].(int64), AccountID: 1, ClientID: "client", FamilyID: args[2].(string), RotatedFrom: args[3].(*string), Resource: args[4].(string), Scope: args[5].(string), AuthEpoch: args[6].(int32), ExpiresAt: args[7].(time.Time), FamilyExpiresAt: args[8].(time.Time), CreatedAt: time.Now(), AccountAuthEpoch: 7}
		tx.tokens[v.TokenHash] = v
		return tokenStructRow(store.OauthToken{TokenHash: v.TokenHash, Kind: v.Kind, GrantID: v.GrantID, AccountID: 1, ClientID: "client", FamilyID: v.FamilyID, RotatedFrom: v.RotatedFrom, Resource: v.Resource, Scope: v.Scope, AuthEpoch: v.AuthEpoch, ExpiresAt: v.ExpiresAt, FamilyExpiresAt: v.FamilyExpiresAt, CreatedAt: v.CreatedAt})
	default:
		return tokenScan{err: fmt.Errorf("unexpected query")}
	}
}
