//go:build integration

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recipeTransport struct{ recipe embedding.Recipe }

func (s *recipeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	var body any
	switch req.URL.Path {
	case "/v1/recipe":
		body = embedding.Report{ProtocolVersion: 1, Recipe: s.recipe, RecipeHash: s.recipe.Hash()}
	case "/v1/embed":
		var input struct {
			RecipeHash string `json:"recipe_hash"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			return nil, err
		}
		if input.RecipeHash != s.recipe.Hash() {
			return nil, errors.New("unexpected query recipe hash")
		}
		values := make([]float32, s.recipe.Dimensions)
		values[0] = 1
		body = map[string]any{"recipe_hash": s.recipe.Hash(), "dimensions": s.recipe.Dimensions,
			"data": []any{map[string]any{"index": 0, "token_count": 1, "embedding": values}}}
	default:
		return nil, fmt.Errorf("unexpected embedder path %s", req.URL.Path)
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return nil, err
	}
	return w.Result(), nil
}

func setQueryRecipe(t *testing.T, db *pgxpool.Pool, query embedding.Recipe) {
	t.Helper()
	document := embedding.DocumentRecipe1()
	dense := retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions,
		Query: query, QueryHash: query.Hash(), Document: document, DocumentHash: document.Hash()}
	raw, err := json.Marshal(dense)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), `UPDATE release SET dense_recipe=$1`, raw); err != nil {
		t.Fatal(err)
	}
}

func TestResolver_DefaultOpenPinsReleaseQueryRecipe(t *testing.T) {
	builder := testpg.NewBuilder(t)
	r, _, dbA, _ := corpusFixtureWithBuilder(t, builder)
	queryA := embedding.QueryRecipe1()
	queryA.RuntimeLibrary = "query runtime A"
	setQueryRecipe(t, dbA, queryA)
	t.Run("switch", func(t *testing.T) {
		_, _, dbB, relB := corpusFixtureWithBuilder(t, builder)
		queryB := queryA
		queryB.RuntimeLibrary = "query runtime B"
		setQueryRecipe(t, dbB, queryB)
		service := &recipeTransport{recipe: queryA}
		client := embedding.NewClient("http://embedder.invalid", &http.Client{Transport: service})
		var pins []string
		r.EmbedderFor = func(hash string) retrieval.Embedder {
			pins = append(pins, hash)
			return embedding.Query{Client: client, RecipeHash: hash}
		}
		t.Cleanup(r.Close)
		assertPrepared := func(lease *Lease, hash string) {
			t.Helper()
			prepared, err := lease.Engine.Prepare(context.Background(), retrieval.Request{Query: "ngân hàng"})
			if err != nil || prepared.Vector == nil || prepared.Vector.RecipeHash != hash {
				t.Fatalf("prepared vector=%+v error=%v", prepared.Vector, err)
			}
		}
		first := takeLease(t, r, "")
		assertPrepared(first, queryA.Hash())
		s := r.Auth.(*fakeStore)
		s.mu.Lock()
		s.active = "corpus_vn_" + relB
		s.mu.Unlock()
		service.recipe = queryB
		second := takeLease(t, r, "")
		assertPrepared(second, queryB.Hash())
		cached := takeLease(t, r, relB)
		if cached.Engine != second.Engine || len(pins) != 2 || pins[0] != queryA.Hash() || pins[1] != queryB.Hash() {
			t.Fatalf("embedder pins=%v", pins)
		}
		first.Done()
	})
}

func TestResolver_DefaultOpenRejectedRecipeIsNotExpired(t *testing.T) {
	r, _, db, _ := corpusFixture(t)
	query := embedding.QueryRecipe1()
	query.RuntimeLibrary = "release query runtime"
	setQueryRecipe(t, db, query)
	service := &recipeTransport{recipe: embedding.QueryRecipe1()}
	client := embedding.NewClient("http://embedder.invalid", &http.Client{Transport: service})
	r.EmbedderFor = func(hash string) retrieval.Embedder {
		return embedding.Query{Client: client, RecipeHash: hash}
	}
	lease, err := r.Resolve(context.Background(), "vn", "")
	if lease != nil {
		defer lease.Done()
	}
	if lease != nil || err == nil || errors.Is(err, ErrExpired) || !strings.Contains(err.Error(), "read embedder recipe failed") {
		t.Fatalf("lease=%+v error=%v", lease, err)
	}
	if len(r.entries) != 0 {
		t.Fatal("failed engine kept its pool cached")
	}
	service.recipe = query
	takeLease(t, r, "")
}
