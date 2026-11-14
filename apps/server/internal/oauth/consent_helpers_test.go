package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const consentBase = "https://law.example"

type consentRow struct {
	values []any
	err    error
}

func (r consentRow) Scan(dst ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dst) != len(r.values) {
		return fmt.Errorf("scan count: got %d want %d", len(dst), len(r.values))
	}
	for i, v := range r.values {
		reflect.ValueOf(dst[i]).Elem().Set(reflect.ValueOf(v))
	}
	return nil
}
func structRow(v any) consentRow {
	value := reflect.ValueOf(v)
	values := make([]any, value.NumField())
	for i := range values {
		values[i] = value.Field(i).Interface()
	}
	return consentRow{values: values}
}

type consentDB struct {
	failQuery        string
	existingGrant    bool
	blocked, deleted bool
	staleSession     bool
	store.DBTX
	mu               sync.Mutex
	client           store.OauthClient
	pending          store.OauthRequest
	code             store.OauthCode
	grants           int64
	pendingCount     int64
	fail, commitFail bool
	claimed          bool
	statements       []string
}

func newConsentDB() *consentDB {
	return &consentDB{client: store.OauthClient{ClientID: "fixture", Kind: "dcr", Name: `Client <script>alert(1)</script>`, RedirectUris: []string{"http://localhost/cb"}, GrantTypes: []string{"authorization_code", "refresh_token"}}}
}
func (d *consentDB) Begin(context.Context) (pgx.Tx, error) {
	d.mu.Lock()
	if d.fail {
		d.mu.Unlock()
		return nil, fmt.Errorf("database unavailable")
	}
	return &consentTx{db: d, pending: d.pending, code: d.code, grants: d.grants, claimed: d.claimed}, nil
}
func (d *consentDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail {
		return consentRow{err: fmt.Errorf("database unavailable")}
	}
	if strings.Contains(sql, "GetClient") {
		return structRow(d.client)
	}
	if len(args) < 2 || args[0] != d.pending.HandleHash || args[1] != d.pending.BrowserHash || d.claimed || !d.pending.ExpiresAt.After(time.Now()) {
		return consentRow{err: pgx.ErrNoRows}
	}
	return consentRow{values: []any{d.pending.ClientID, d.pending.RedirectUri, d.pending.State, d.pending.CodeChallenge, d.pending.Resource, d.pending.Scope}}
}

type consentTx struct {
	pgx.Tx
	db              *consentDB
	pending         store.OauthRequest
	code            store.OauthCode
	grants          int64
	claimed, closed bool
}

func (tx *consentTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.db.statements = append(tx.db.statements, sql)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (tx *consentTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if tx.db.failQuery != "" && strings.Contains(sql, tx.db.failQuery) {
		return consentRow{err: fmt.Errorf("query failed")}
	}
	switch {
	case strings.Contains(sql, "GetClient"):
		return structRow(tx.db.client)
	case strings.Contains(sql, "CountPendingRequests"):
		return consentRow{values: []any{tx.db.pendingCount}}
	case strings.Contains(sql, "CreateRequest"):
		tx.pending = store.OauthRequest{HandleHash: args[0].(string), BrowserHash: args[1].(string), ClientID: args[2].(string), RedirectUri: args[3].(string), State: args[4].(string), CodeChallenge: args[5].(string), Resource: args[6].(string), Scope: args[7].(string), ExpiresAt: args[8].(time.Time)}
		return structRow(tx.pending)
	case strings.Contains(sql, "LockCredentialAccount"):
		account := store.Account{ID: 7, AuthEpoch: 3}
		now := time.Now()
		if tx.db.blocked {
			account.BlockedAt = &now
		}
		if tx.db.deleted {
			account.DeleteRequestedAt = &now
		}
		return structRow(account)
	case strings.Contains(sql, "FROM web_session"):
		if tx.db.staleSession {
			return consentRow{err: pgx.ErrNoRows}
		}
		return structRow(store.GetLiveAuthoritySessionRow{ID: 1, AuthEpoch: 3})
	case strings.Contains(sql, "ClaimRequest"):
		if tx.claimed || args[0] != tx.pending.HandleHash || args[1] != tx.pending.BrowserHash || !tx.pending.ExpiresAt.After(time.Now()) {
			return consentRow{err: pgx.ErrNoRows}
		}
		tx.claimed = true
		tx.pending.AccountID = args[2].(*int64)
		return structRow(tx.pending)
	case strings.Contains(sql, "UpsertGrant"):
		if !tx.db.existingGrant {
			tx.grants++
		}
		return structRow(store.OauthGrant{ID: 9, AccountID: 7, ClientID: tx.pending.ClientID})
	case strings.Contains(sql, "CountLiveGrants"):
		return consentRow{values: []any{tx.grants}}
	case strings.Contains(sql, "CreateCode"):
		tx.code = store.OauthCode{CodeHash: args[0].(string), RedirectUri: args[1].(string), Resource: args[2].(string), CodeChallenge: args[3].(string), Scope: args[4].(string), AuthEpoch: args[5].(int32), ExpiresAt: args[6].(time.Time), GrantID: args[7].(int64), AccountID: 7, ClientID: tx.pending.ClientID}
		return structRow(tx.code)
	default:
		return consentRow{err: fmt.Errorf("unexpected query")}
	}
}
func (tx *consentTx) Commit(context.Context) error {
	if tx.closed {
		return pgx.ErrTxClosed
	}
	tx.closed = true
	defer tx.db.mu.Unlock()
	if tx.db.commitFail {
		return fmt.Errorf("unknown commit outcome")
	}
	tx.db.pending = tx.pending
	tx.db.code = tx.code
	tx.db.grants = tx.grants
	tx.db.claimed = tx.claimed
	return nil
}
func (tx *consentTx) Rollback(context.Context) error {
	if tx.closed {
		return pgx.ErrTxClosed
	}
	tx.closed = true
	tx.db.mu.Unlock()
	return nil
}

type consentSessions struct{ websession.Store }

func (s consentSessions) GetSession(context.Context, string) (store.GetSessionRow, error) {
	now := time.Now()
	return store.GetSessionRow{ID: 1, AccountID: 7, CsrfSecret: "session-secret-fixture", CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour), SessionAuthEpoch: 3, AccountAuthEpoch: 3}, nil
}
func consentSessionManager() *websession.Manager {
	return &websession.Manager{Store: consentSessions{}, CacheTTL: -1}
}
func authorizeValues() url.Values {
	return url.Values{"client_id": {"fixture"}, "redirect_uri": {"http://localhost/cb"}, "response_type": {"code"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(make([]byte, 32))}, "code_challenge_method": {"S256"}, "state": {"opaque & state"}, "resource": {consentBase + "/mcp"}}
}
func authorizeFor(t *testing.T, d *consentDB, v url.Values) *httptest.ResponseRecorder {
	t.Helper()
	h := NewAuthorizeHandler(consentBase, d, func(context.Context, string) (Client, error) {
		if v.Get("client_id") != d.client.ClientID {
			return Client{}, ErrInvalidClient
		}
		return clientFromRow(d.client), nil
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/oauth/authorize?"+v.Encode(), nil))
	return w
}
func pendingConsent(t *testing.T, d *consentDB) (string, *http.Cookie) {
	t.Helper()
	w := authorizeFor(t, d, authorizeValues())
	if w.Code != 303 {
		t.Fatalf("authorize: %d", w.Code)
	}
	return w.Header().Get("Location"), w.Result().Cookies()[0]
}
func consentRequest(path, method string, cookie *http.Cookie, session bool, form url.Values) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if session {
		r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	}
	if method == "POST" {
		r.Header.Set("Origin", consentBase)
		r.Header.Set("Referer", consentBase+strings.Split(path, "?")[0])
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Sec-Fetch-Mode", "navigate")
		r.Header.Set("Sec-Fetch-Dest", "document")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return r
}
func formFor(path, decision string) url.Values {
	return url.Values{"decision": {decision}, "csrf_token": {consentFormToken("session-secret-fixture", strings.TrimPrefix(strings.Split(path, "?")[0], "/oauth/consent/"))}}
}
func consentResponse(d *consentDB, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	NewConsentHandler(consentBase, d, consentSessionManager()).ServeHTTP(w, r)
	return w
}
func consentHash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
