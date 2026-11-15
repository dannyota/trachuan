package retrieval

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpenDraft_SchemaVersions(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	hash := strings.Repeat("a", 64)
	reached := errors.New("schema accepted")
	for _, version := range []int{3, 4, 5, 6, 7, 8} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			release := Release{Country: "vn", Rel: "20990101_1", SchemaVersion: version, RetrievalRecipe: Recipe2(), LexicalRecipe: lexical.Recipe1(), ChunkRecipe: json.RawMessage(`{}`), EvaluationSummary: json.RawMessage(`{}`), SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, BuiltAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), LexicalStats: lexical.StatsSummary{Version: 1, AvgDL: 1, VocabularySHA256: hash, DFSHA256: hash}}
			_, err := OpenDraft(t.Context(), pool, failingRecipeEmbedder{reached}, release, Options{Now: time.Now, Profile: "serving"})
			if version >= 4 && version <= 7 {
				if !errors.Is(err, reached) {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != "invalid release metadata" {
				t.Fatalf("unsupported schema %d: %v", version, err)
			}
		})
	}
}
