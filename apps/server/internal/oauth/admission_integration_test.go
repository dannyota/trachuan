//go:build integration

package oauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func admissionDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()
	dsn := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, `INSERT INTO oauth_client(client_id,kind,name,redirect_uris,last_used_at) SELECT 'client-'||i,'dcr','Fixture',ARRAY['http://localhost/cb'],now()-i*interval '1 second' FROM generate_series(1,10000) i`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func admissionTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func protectRequest(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := store.New(tx).CreateRequest(ctx, store.CreateRequestParams{
		HandleHash: fmt.Sprintf("%x", sha256.Sum256([]byte(id))), BrowserHash: strings.Repeat("a", 64), ClientID: id,
		RedirectUri: "http://localhost/cb", CodeChallenge: "challenge", Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	return err
}

func TestAdmission_CapEvictsLeastRecentNotInUse(t *testing.T) {
	pool := admissionDB(t)
	ctx := t.Context()
	tx := admissionTx(t, pool)
	if _, err := InUse(ctx, tx, "client-10000"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE oauth_client SET last_used_at=now()-interval '1 day' WHERE client_id='client-10000'`); err != nil {
		t.Fatal(err)
	}
	if err := protectRequest(ctx, tx, "client-10000"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx = admissionTx(t, pool)
	c, err := Admit(ctx, tx, Client{ID: "new", Kind: "dcr", Name: "New", RedirectURIs: []string{"http://localhost/cb"}})
	if err != nil || c.ID != "new" {
		t.Fatalf("admit %v %v", c, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	q := store.New(pool)
	if _, err := q.GetClient(ctx, "client-9999"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("LRU survived: %v", err)
	}
	if _, err := q.GetClient(ctx, "client-10000"); err != nil {
		t.Fatal("in-use client evicted")
	}
	if count, err := q.CountClients(ctx); err != nil || count != 10000 {
		t.Fatalf("count %d err %v", count, err)
	}
	// Touching twice inside one hour must not write twice.
	tx = admissionTx(t, pool)
	if _, err := InUse(ctx, tx, "client-9998"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE oauth_client SET last_used_at=now()-interval '30 minutes' WHERE client_id='client-9998'`); err != nil {
		t.Fatal(err)
	}
	var first time.Time
	if err := tx.QueryRow(ctx, `SELECT last_used_at FROM oauth_client WHERE client_id='client-9998'`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if _, err := InUse(ctx, tx, "client-9998"); err != nil {
		t.Fatal(err)
	}
	var second time.Time
	if err := tx.QueryRow(ctx, `SELECT last_used_at FROM oauth_client WHERE client_id='client-9998'`).Scan(&second); err != nil || !first.Equal(second) {
		t.Fatalf("hourly touch: %v", err)
	}
	if _, err := InUse(ctx, tx, "missing"); !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("missing row: %v", err)
	}
}

func TestAdmission_NeverEvictsInUse(t *testing.T) {
	pool := admissionDB(t)
	ctx := t.Context()
	// Keep a shared lock across the client read and creation of authority.
	authorizing := admissionTx(t, pool)
	if _, err := InUse(ctx, authorizing, "client-10000"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := range 8 {
		wg.Go(func() {
			tx, err := pool.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			defer func() { _ = tx.Rollback(ctx) }()
			_, err = Admit(ctx, tx, Client{ID: fmt.Sprintf("new-%d", i), Kind: "dcr", Name: "New", RedirectURIs: []string{"http://localhost/cb"}})
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- err
		})
	}
	// PostgreSQL reports actual lock waiters, avoiding a timing-based race test.
	deadline := time.Now().Add(2 * time.Second)
	for {
		var waiting bool
		if err := authorizing.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_locks WHERE locktype='advisory' AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("admission did not wait for the shared client lock")
		}
	}
	if _, err := authorizing.Exec(ctx, `UPDATE oauth_client SET last_used_at=now()-interval '1 day' WHERE client_id='client-10000'`); err != nil {
		t.Fatal(err)
	}
	if err := protectRequest(ctx, authorizing, "client-10000"); err != nil {
		t.Fatal(err)
	}
	if err := authorizing.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.New(pool).GetClient(ctx, "client-10000"); err != nil {
		t.Fatal("authorization client evicted")
	}
	count, err := store.New(pool).CountClients(ctx)
	if err != nil || count != 10000 {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestAdmission_AllInUseIs503(t *testing.T) {
	pool := admissionDB(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_request(handle_hash,browser_hash,client_id,redirect_uri,state,code_challenge,resource,scope,expires_at) SELECT encode(sha256(client_id::bytea),'hex'),repeat('a',64),client_id,'http://localhost/cb','','challenge','https://example.invalid/mcp','mcp:read',now()+interval '10 minutes' FROM oauth_client`); err != nil {
		t.Fatal(err)
	}
	clients := NewClients(pool, nil, nil)
	if _, err := clients.Register(ctx, Client{ID: "new", Kind: "dcr", RedirectURIs: []string{"http://localhost/cb"}}); !errors.Is(err, ErrTemporarilyUnavailable) {
		t.Fatalf("all in use error %v", err)
	}
	// Existing admissions remain available at the cap and do not evict.
	if _, err := clients.Register(ctx, Client{ID: "client-1", Kind: "dcr", RedirectURIs: []string{"http://localhost/cb"}}); err != nil {
		t.Fatal(err)
	}
	w := newRegistrationResponse(t, NewRegisterHandler(clients.Register))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"temporarily_unavailable"`) {
		t.Fatalf("response %d %s", w.Code, w.Body)
	}
}

func newRegistrationResponse(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(`{"redirect_uris":["http://localhost/cb"]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAdmission_ProtectsCodesAndFamilies(t *testing.T) {
	pool := admissionDB(t)
	ctx := t.Context()
	q := store.New(pool)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "admission-fixture", Email: "fixture@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"client-10000", "client-9999"} {
		tx := admissionTx(t, pool)
		if _, err := InUse(ctx, tx, id); err != nil {
			t.Fatal(err)
		}
		queries := store.New(tx)
		grant, err := queries.UpsertGrant(ctx, store.UpsertGrantParams{AccountID: account.ID, ClientID: id, Resource: "https://example.invalid/mcp", Scope: "mcp:read"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			_, err = queries.CreateCode(ctx, store.CreateCodeParams{GrantID: grant.ID, CodeHash: strings.Repeat("c", 64), RedirectUri: "http://localhost/cb", CodeChallenge: "challenge", Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: time.Now().Add(time.Minute)})
		} else {
			_, err = queries.CreateTokenFamily(ctx, store.CreateTokenFamilyParams{GrantID: grant.ID, TokenHash: strings.Repeat("d", 64), FamilyID: strings.Repeat("e", 64), Kind: "refresh", Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: time.Now().Add(-time.Minute), FamilyExpiresAt: time.Now().Add(24 * time.Hour)})
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE oauth_client SET last_used_at=now()-interval '1 day' WHERE client_id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewClients(pool, nil, nil).Register(ctx, Client{ID: "new", Kind: "dcr", RedirectURIs: []string{"http://localhost/cb"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"client-10000", "client-9999"} {
		if _, err := q.GetClient(ctx, id); err != nil {
			t.Fatalf("authority client %s evicted: %v", id, err)
		}
	}
	if _, err := q.GetClient(ctx, "client-9998"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("eligible LRU retained: %v", err)
	}
}

func TestAdmission_CIMDRefreshPersistsMetadata(t *testing.T) {
	pool := admissionDB(t)
	ctx := t.Context()
	var version atomic.Int32
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	f := tlsResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		name, redirect := "Old", "https://old.example/cb"
		if version.Load() != 0 {
			name, redirect = "New", "https://new.example/cb"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"client_id":"https://example.com/client","client_name":%q,"redirect_uris":[%q]}`, name, redirect)
	})
	f.clock = func() time.Time { return now }
	clients := NewClients(pool, nil, f)
	old, err := clients.Resolve(ctx, "https://example.com/client")
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.New(pool).GetClient(ctx, old.ID)
	if err != nil || row.MetadataSha256 == nil {
		t.Fatalf("initial row: %v", err)
	}
	oldDigest := *row.MetadataSha256
	version.Store(1)
	now = now.Add(24 * time.Hour)
	updated, err := clients.Resolve(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "New" || !slices.Equal(updated.RedirectURIs, []string{"https://new.example/cb"}) {
		t.Fatalf("resolved metadata: %v", updated)
	}
	tx := admissionTx(t, pool)
	inUse, err := InUse(ctx, tx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err = store.New(tx).GetClient(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inUse.Name != updated.Name || !slices.Equal(inUse.RedirectURIs, updated.RedirectURIs) || row.Name != updated.Name || !slices.Equal(row.RedirectUris, updated.RedirectURIs) {
		t.Fatalf("stored metadata differs: %v %v", inUse, row)
	}
	if row.MetadataSha256 == nil || *row.MetadataSha256 == oldDigest {
		t.Fatal("metadata digest did not change")
	}
	if !row.CreatedAt.Equal(old.CreatedAt) || !updated.CreatedAt.Equal(old.CreatedAt) {
		t.Fatal("client issuance time changed")
	}
}

func TestAdmission_PreregisteredChangePersistsMetadata(t *testing.T) {
	pool := admissionDB(t)
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "clients.json")
	load := func(name, redirect string) []Client {
		t.Helper()
		raw := fmt.Sprintf(`[{"client_id":"pre-client","client_name":%q,"redirect_uris":[%q]}]`, name, redirect)
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		configured, err := LoadPreregistered(path)
		if err != nil {
			t.Fatal(err)
		}
		return configured
	}
	clients := NewClients(pool, load("Old", "https://old.example/cb"), nil)
	first, err := clients.Resolve(ctx, "pre-client")
	if err != nil {
		t.Fatal(err)
	}
	clients = NewClients(pool, load("New", "https://new.example/cb"), nil)
	updated, err := clients.Resolve(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	tx := admissionTx(t, pool)
	inUse, err := InUse(ctx, tx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.New(tx).GetClient(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "New" || inUse.Name != "New" || row.Name != "New" || !slices.Equal(inUse.RedirectURIs, []string{"https://new.example/cb"}) || !slices.Equal(row.RedirectUris, updated.RedirectURIs) {
		t.Fatalf("metadata not refreshed: %v %v %v", updated, inUse, row)
	}
	if row.MetadataSha256 != nil || !row.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("preregistered identity changed")
	}
}
