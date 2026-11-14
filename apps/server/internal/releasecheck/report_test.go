package releasecheck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
)

type recipeTransport struct{ report embedding.Report }

func (r recipeTransport) RoundTrip(*http.Request) (*http.Response, error) {
	raw, err := json.Marshal(r.report)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
}

func TestDescribe_ReportsPinnedAndRunningRecipesSeparately(t *testing.T) {
	for _, recipe := range []embedding.Recipe{embedding.QueryRecipe1(), embedding.TestRecipe()} {
		client := embedding.NewClient("http://embedder", &http.Client{Transport: recipeTransport{embedding.Report{ProtocolVersion: 1, RecipeHash: recipe.Hash(), Recipe: recipe}}})
		got, err := Describe(context.Background(), client)
		if err != nil {
			t.Fatal(err)
		}
		if got.DenseQueryHash != embedding.QueryRecipe1().Hash() || got.RunningQuery.Recipe != recipe || got.LexicalRecipe != lexical.Recipe1() || len(got.RetrievalRecipes) != 2 || len(got.SchemaVersions) != len(corpusdb.SupportedSchemaVersions()) {
			t.Fatalf("unexpected description: %+v", got)
		}
	}
}

func TestDatabaseConfig_RequiresCorpusReaderAndReadOnly(t *testing.T) {
	for _, raw := range []string{"postgres://postgres@localhost/{database}", "postgres://corpus_reader@localhost/{database}?user=postgres", "invalid"} {
		if _, err := databaseConfig(raw, "corpus_vn_20990101_1"); err == nil {
			t.Fatal("accepted unsafe database configuration")
		}
	}
	cfg, err := databaseConfig("postgres://corpus_reader@localhost/{database}?default_transaction_read_only=off", "corpus_vn_20990101_1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] != "on" || cfg.ConnConfig.Database != "corpus_vn_20990101_1" {
		t.Fatal("missing read-only database binding")
	}
}
