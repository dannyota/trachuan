//go:build integration

package release

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
	read, _, err := r.Resolve(ctx, "vn", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, searchErr := read.Search(ctx, retrieval.Request{Query: "ngân hàng"}); !errors.Is(searchErr, retrieval.ErrSearchUnavailable) {
		t.Fatalf("read-only search error=%v", searchErr)
	}
	name := "corpus_vn_" + rel
	pool := r.entries[name].pool
	search, _, err := r.Resolve(ctx, "vn", rel, true)
	if err != nil {
		t.Fatal(err)
	}
	if search.Release().Rel != rel || r.entries[name].pool != pool || pool.Config().MaxConns != 4 {
		t.Fatal("release or pool not pinned")
	}
	r.Close()
	// A new resolver must reject a corpus whose row claims another release.
	r2 := &Resolver{Auth: r.Auth, Clock: r.Clock, URLTemplate: r.URLTemplate, Embedder: r.Embedder}
	t.Cleanup(r2.Close)
	if _, updateErr := db.Exec(ctx, `UPDATE release SET rel='20990101_invalid'`); updateErr != nil {
		t.Fatal(updateErr)
	}
	if _, _, resolveErr := r2.Resolve(ctx, "vn", "", false); resolveErr == nil {
		t.Fatal("accepted mismatched release identity")
	}
}
func TestResolver_DefaultOpenIncompatibleSearchStillReadable(t *testing.T) {
	for _, arm := range []string{"dense", "lexical"} {
		t.Run(arm, func(t *testing.T) {
			r, _, db, rel := corpusFixture(t)
			if arm == "dense" {
				recipe := embedding.QueryRecipe1()
				recipe.Runtime = "other-runtime"
				r.Embedder = recipeEmbedder{recipe: recipe}
			} else {
				if _, err := db.Exec(context.Background(), `UPDATE release SET lexical_recipe=jsonb_set(lexical_recipe,'{tokenizer}','"older-tokenizer"')`); err != nil {
					t.Fatal(err)
				}
			}
			_, _, err := r.Resolve(context.Background(), "vn", rel, true)
			if !errors.Is(err, ErrIncompatible) {
				t.Fatalf("search error=%v", err)
			}
			e, _, err := r.Resolve(context.Background(), "vn", rel, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Status(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestResolver_DefaultOpenPrunedDatabaseIsExpired(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "first_open", true: "cached"}[cached], func(t *testing.T) {
			r, admin, db, rel := corpusFixture(t)
			store := r.Auth.(*fakeStore)
			store.active = ""
			store.retained = []RetainedRelease{{DBName: "corpus_vn_" + rel, RetainUntil: r.Clock().Add(time.Hour)}}
			if cached {
				if _, _, err := r.Resolve(context.Background(), "vn", rel, true); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			if !testpg.DropCorpusDatabase(t, admin, "corpus_vn_"+rel, "vn/"+rel) {
				t.Fatal("failed marker-checked prune")
			}
			_, _, err := r.Resolve(context.Background(), "vn", rel, true)
			if !errors.Is(err, ErrExpired) {
				t.Fatalf("pruned release error=%v", err)
			}
		})
	}
}
