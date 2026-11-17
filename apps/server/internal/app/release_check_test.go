package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/releasecheck"
	"danny.vn/trachuan/internal/embedding"
)

func TestReleaseCheck_RejectsArgumentsBeforeLoadingServeConfig(t *testing.T) {
	for _, args := range [][]string{{"--database"}, {"--database", "auth"}, {"extra"}} {
		var output bytes.Buffer
		err := run(context.Background(), append([]string{"release-check"}, args...), &output, &output)
		if err == nil || !strings.Contains(err.Error(), "release-check arguments") {
			t.Fatalf("args %v: %v", args, err)
		}
	}
}

func TestReleaseCheck_FrameworksAndLawReports(t *testing.T) {
	recipe := embedding.QueryRecipe1()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(embedding.Report{ProtocolVersion: 1, RecipeHash: recipe.Hash(), Recipe: recipe})
	}))
	defer server.Close()
	t.Setenv("SERVER_QUERY_EMBEDDER_URL", server.URL)
	var out bytes.Buffer
	if err := runReleaseCheck(t.Context(), []string{"--kind", "frameworks"}, &out); err != nil {
		t.Fatal(err)
	}
	want := `{"schema_versions":[1],"retrieval_recipe_versions":[2],"lexical_recipe_versions":[1],"query_recipe_hash":"` + recipe.Hash() + `"}` + "\n"
	if out.String() != want {
		t.Fatalf("frameworks report: %s", out.String())
	}
	out.Reset()
	if err := runReleaseCheck(t.Context(), nil, &out); err != nil {
		t.Fatal(err)
	}
	client := embedding.NewClient(server.URL, server.Client())
	law, err := releasecheck.Describe(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	var original bytes.Buffer
	if err = json.NewEncoder(&original).Encode(law); err != nil {
		t.Fatal(err)
	}
	if out.String() != original.String() {
		t.Fatal("law command bytes changed")
	}
}

func TestReleaseCheck_RejectsKindAndDatabaseMismatch(t *testing.T) {
	for _, args := range [][]string{
		{"--kind", "law"}, {"--kind", "frameworks", "--kind", "frameworks"},
		{"--kind", "frameworks", "--database", "corpus_vn_20990101"},
		{"--database", "frameworks_20990101"}, {"--kind"},
	} {
		var out bytes.Buffer
		if err := runReleaseCheck(t.Context(), args, &out); err == nil || !strings.Contains(err.Error(), "release-check arguments") {
			t.Fatalf("args %v: %v", args, err)
		}
	}
}
