package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpen_ClosedPoolDuringOpeningIsReleaseGone(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close() // Never connect to PostgreSQL.
	hash := strings.Repeat("a", 64)
	release := Release{Country: "vn", Rel: "20990101", SchemaVersion: corpusdb.SchemaVersion, RetrievalRecipe: Recipe2(), LexicalRecipe: lexical.Recipe1(), ChunkRecipe: json.RawMessage(`{}`), EvaluationSummary: json.RawMessage(`{}`), SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, BuiltAt: time.Now(), LexicalStats: lexical.StatsSummary{Version: 1, AvgDL: 1, VocabularySHA256: hash, DFSHA256: hash}}
	for _, stage := range []string{"metadata", "dimensions"} {
		t.Run(stage, func(t *testing.T) {
			var err error
			if stage == "metadata" {
				_, err = Open(context.Background(), pool, &fakeEmbedder{}, Options{Now: time.Now, Profile: "serving"})
			} else {
				_, err = OpenDraft(context.Background(), pool, &fakeEmbedder{}, release, Options{Now: time.Now, Profile: "serving"})
			}
			if !errors.Is(err, ErrReleaseGone) || strings.Contains(err.Error(), "closed pool") {
				t.Fatal("opening failure lost its safe release-gone category", err)
			}
		})
	}
}

type dimensionErrorDB struct{ cause error }

func (db dimensionErrorDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return dimensionErrorRow(db)
}

type dimensionErrorRow struct{ cause error }

func (row dimensionErrorRow) Scan(...any) error { return row.cause }

func TestOpen_DimensionErrorsAreClassifiedAndSanitized(t *testing.T) {
	for _, code := range []string{"3D000", "57P01", "57P04", "08006"} {
		cause := &pgconn.PgError{Code: code, Message: "private database details"}
		_, err := readEmbeddingDimensions(t.Context(), dimensionErrorDB{cause: cause})
		want := ErrReleaseGone
		if code == "08006" {
			want = ErrUnavailable
		}
		if !errors.Is(err, want) || err.Error() != "read embedding dimensions failed" || errors.Is(err, cause) {
			t.Fatal("unsafe or unclassified dimension failure", code, err)
		}
	}
}

type failingRecipeEmbedder struct{ cause error }

func (f failingRecipeEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.Recipe{}, f.cause
}
func (f failingRecipeEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	return embedding.Vector{}, nil
}

func TestOpenDraft_WrapsRecipeFailure(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	hash := strings.Repeat("a", 64)
	release := Release{Country: "vn", Rel: "20990101", SchemaVersion: corpusdb.SchemaVersion, RetrievalRecipe: Recipe2(), LexicalRecipe: lexical.Recipe1(), ChunkRecipe: json.RawMessage(`{}`), EvaluationSummary: json.RawMessage(`{}`), SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, BuiltAt: time.Now(), LexicalStats: lexical.StatsSummary{Version: 1, AvgDL: 1, VocabularySHA256: hash, DFSHA256: hash}}
	cause := errors.New("query recipe mismatch: details")
	_, err = OpenDraft(context.Background(), pool, failingRecipeEmbedder{cause}, release, Options{Now: time.Now, Profile: "serving"})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "read embedder recipe failed: query recipe mismatch: details") {
		t.Fatalf("recipe error = %v", err)
	}
}

func TestOpenDraft_ReleaseRecipePairs(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	hash := strings.Repeat("a", 64)
	release := Release{Country: "vn", Rel: "20990101", SchemaVersion: corpusdb.SchemaVersion, LexicalRecipe: lexical.Recipe1(), ChunkRecipe: json.RawMessage(`{}`), EvaluationSummary: json.RawMessage(`{}`), SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, BuiltAt: time.Now(), LexicalStats: lexical.StatsSummary{Version: 1, AvgDL: 1, VocabularySHA256: hash, DFSHA256: hash}}
	reached := errors.New("metadata accepted")
	for _, tc := range []struct {
		version int
		merge   string
		valid   bool
	}{{2, "", true}, {3, "article-clauses/1", true}, {3, "", false}, {2, "article-clauses/1", false}, {1, "", false}, {4, "article-clauses/1", false}} {
		release.RetrievalRecipe = Recipe2()
		release.RetrievalRecipe.Version, release.RetrievalRecipe.SiblingMerge = tc.version, tc.merge
		_, err = OpenDraft(context.Background(), pool, failingRecipeEmbedder{reached}, release, Options{Now: time.Now, Profile: "serving"})
		if tc.valid {
			if !errors.Is(err, reached) {
				t.Fatalf("recipe %d %q: %v", tc.version, tc.merge, err)
			}
		} else if err == nil || err.Error() != "invalid release metadata" {
			t.Fatalf("accepted recipe %d %q: %v", tc.version, tc.merge, err)
		}
	}
}
