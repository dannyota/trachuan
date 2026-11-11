//go:build integration

package release

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recipeEmbedder struct{ recipe embedding.Recipe }

func (e recipeEmbedder) Recipe(context.Context) (embedding.Recipe, error) { return e.recipe, nil }
func (recipeEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	return embedding.Vector{}, errors.New("embedding not used by resolver tests")
}

func corpusFixture(t *testing.T) (*Resolver, *pgx.Conn, *pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	admin, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal("connect test builder")
	}
	t.Cleanup(func() {
		if closeErr := admin.Close(ctx); closeErr != nil {
			t.Error(closeErr)
		}
	})
	rel := testpg.Release(t)
	name := "corpus_vn_" + rel
	if _, createErr := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{builder.Template}.Sanitize()); createErr != nil {
		t.Fatal(createErr)
	}
	marker := `{"run_key":"vn/` + rel + `"}`
	if _, commentErr := admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '"+marker+"'"); commentErr != nil {
		t.Fatal(commentErr)
	}
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, "vn/"+rel) })
	dbURL := strings.Replace(builder.URL, "/postgres", "/"+name, 1)
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal("open test corpus")
	}
	t.Cleanup(db.Close)
	if applyErr := corpusdb.Apply(ctx, db, 1024); applyErr != nil {
		t.Fatal(applyErr)
	}
	query, document := embedding.QueryRecipe1(), embedding.DocumentRecipe1()
	dense := retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Query: query, QueryHash: query.Hash(), Document: document, DocumentHash: document.Hash()}
	hash := strings.Repeat("a", 64)
	recipe := retrieval.Recipe1()
	threshold := 0.1
	recipe.Abstention.Threshold = &threshold
	recipe.Abstention.CalibrationSHA256 = hash
	stats := lexical.StatsSummary{Version: 1, AvgDL: 1, VocabularySHA256: hash, DFSHA256: hash}
	marshal := func(v any) []byte {
		value, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return value
	}
	_, err = db.Exec(ctx, `INSERT INTO release VALUES(true,'vn',$1,3,'v1',$2,'{}',$3,$4,$5,$6,$6,$6,'{}',now())`, rel, marshal(dense), marshal(lexical.Recipe1()), marshal(recipe), marshal(stats), hash)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	r := &Resolver{Auth: &fakeStore{active: name}, Clock: func() time.Time { return now }, URLTemplate: strings.Replace(builder.URL, "/postgres", "/{database}", 1), Embedder: recipeEmbedder{recipe: query}}
	t.Cleanup(r.Close)
	return r, admin, db, rel
}

func TestResolver_DefaultOpenSharesPoolAndChecksIdentity(t *testing.T) {
	r, _, db, rel := corpusFixture(t)
	ctx := context.Background()
	read, _, err := r.Resolve(ctx, "vn", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "corpus_vn_" + rel
	pool := r.entries[name].pool
	search, _, err := r.Resolve(ctx, "vn", rel)
	if err != nil {
		t.Fatal(err)
	}
	if search != read || search.Release().Rel != rel || r.entries[name].pool != pool || pool.Config().MaxConns != 4 {
		t.Fatal("release or pool not pinned")
	}
	r.Close()
	// A new resolver must reject a corpus whose row claims another release.
	r2 := &Resolver{Auth: r.Auth, Clock: r.Clock, URLTemplate: r.URLTemplate, Embedder: r.Embedder}
	t.Cleanup(r2.Close)
	if _, updateErr := db.Exec(ctx, `UPDATE release SET rel='20990101_invalid'`); updateErr != nil {
		t.Fatal(updateErr)
	}
	if _, _, resolveErr := r2.Resolve(ctx, "vn", ""); resolveErr == nil {
		t.Fatal("accepted mismatched release identity")
	}
}
func TestResolver_DefaultOpenMissingDatabaseIsExpired(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "first_open", true: "cached"}[cached], func(t *testing.T) {
			r, admin, db, rel := corpusFixture(t)
			if cached {
				if _, _, err := r.Resolve(context.Background(), "vn", rel); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			if !testpg.DropCorpusDatabase(t, admin, "corpus_vn_"+rel, "vn/"+rel) {
				t.Fatal("failed marker-checked removal")
			}
			_, _, err := r.Resolve(context.Background(), "vn", rel)
			if !errors.Is(err, ErrExpired) {
				t.Fatalf("missing database error=%v", err)
			}
		})
	}
}

// authStore exposes only the active pointer from the generated store.
type authStore struct{ queries *store.Queries }

func (s authStore) GetActiveRelease(ctx context.Context, country string) (string, error) {
	row, err := s.queries.GetActiveRelease(ctx, country)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return row.ActiveDb, err
}

func TestResolver_PinnedRetiredRowIsExpired(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal("connect test auth database")
	}
	t.Cleanup(func() {
		if err := db.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.Exec(ctx, `INSERT INTO corpus_release VALUES ('vn','corpus_vn_20990103',now(),'operator');
INSERT INTO retired_release VALUES ('vn','corpus_vn_20990102',now(),now()+interval '7 days',NULL)`); err != nil {
		t.Fatal(err)
	}
	r, _, _ := resolverFixture()
	r.Auth = authStore{queries: store.New(db)}
	r.Open = func(string) (*retrieval.Engine, error) {
		t.Error("opened non-active database")
		return nil, nil
	}
	t.Cleanup(r.Close)
	got, rel, err := r.Resolve(ctx, "vn", "20990102")
	if !errors.Is(err, ErrExpired) || got != nil || rel != "" {
		t.Fatalf("engine=%p release=%q error=%v", got, rel, err)
	}
}

func TestResolver_RemovingActiveDrainsPool(t *testing.T) {
	r, _, _, rel := corpusFixture(t)
	ctx := context.Background()
	if _, _, err := r.Resolve(ctx, "vn", rel); err != nil {
		t.Fatal(err)
	}
	entry := r.entries["corpus_vn_"+rel]
	conn, err := entry.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	closing := make(chan struct{})
	entry.closePool = func() {
		close(closing)
		entry.pool.Close()
	}
	s := r.Auth.(*fakeStore)
	s.mu.Lock()
	s.active = ""
	s.mu.Unlock()
	result := make(chan error, 1)
	go func() { _, _, err := r.Resolve(ctx, "vn", rel); result <- err }()
	<-closing
	select {
	case err := <-result:
		t.Fatalf("closed with an in-flight connection: %v", err)
	default:
	}
	if _, err := conn.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatalf("in-flight connection failed: %v", err)
	}
	conn.Release()
	if err := <-result; !errors.Is(err, ErrExpired) {
		t.Fatalf("removed active release error=%v", err)
	}
	if unexpected, err := entry.pool.Acquire(ctx); err == nil {
		unexpected.Release()
		t.Fatal("old pool still accepts connections")
	}
}
