//go:build integration

package release

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFrameworksPool_RefusesUnsafeConnectionsAndEngine(t *testing.T) {
	raw := testpg.NewDatabase(t)
	admin, err := pgxpool.New(t.Context(), raw)
	if err != nil {
		t.Fatal("connect invented database")
	}
	defer admin.Close()
	var name string
	if err = admin.QueryRow(t.Context(), `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	template := strings.Replace(raw, "/"+name, "/{database}", 1)
	cfg, err := frameworksPoolConfig(template, name)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if conn, acquireErr := pool.Acquire(ctx); acquireErr == nil {
		conn.Release()
		t.Fatal("unsafe connection entered pool")
	}
	if pool.Stat().TotalConns() != 0 {
		t.Fatal("unsafe connection retained")
	}
	r := &Resolver{URLTemplate: template, Frameworks: &serve.Options{CursorKey: []byte(strings.Repeat("s", 32))}, EmbedderFor: func(string, embedding.Recipe) retrieval.Embedder { return embedding.Query{} }}
	entry := &cacheEntry{}
	r.entries = map[string]*cacheEntry{name: entry}
	defer r.Close()
	ctx2, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	if err = r.openFrameworks(ctx2, name, entry); err != serve.ErrUnavailable || entry.frameworks != nil {
		t.Fatal("unsafe database opened a frameworks engine")
	}
}
