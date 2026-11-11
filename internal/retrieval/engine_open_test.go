package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
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
	release := Release{Country: "vn", Rel: "20990101", SchemaVersion: corpusdb.SchemaVersion, RetrievalRecipe: Recipe1(), LexicalRecipe: lexical.Recipe1(), ChunkRecipe: json.RawMessage(`{}`), EvaluationSummary: json.RawMessage(`{}`), SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, BuiltAt: time.Now(), LexicalStats: lexical.StatsSummary{Version: 1, AvgDL: 1, VocabularySHA256: hash, DFSHA256: hash}}
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
