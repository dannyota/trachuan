package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

func TestReleaseRowCLI_ValidatesBeforeLoadingAndDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.json")
	calls := 0
	load := func(context.Context, string, string) ([]byte, error) {
		calls++
		return []byte(`{"run":{"complete":true}}`), nil
	}
	for _, args := range [][]string{nil, {"vn", "20990101_1"}, {"../", "20990101_1", "--out", path}, {"vn", "bad", "--out", path}} {
		var errout bytes.Buffer
		if code := runReleaseRowWith(context.Background(), args, &errout, load); code != 2 || calls != 0 {
			t.Fatalf("args=%v code=%d calls=%d stderr=%s", args, code, calls, &errout)
		}
	}
	args := []string{"vn", "20990101_1", "--out", path}
	var errout bytes.Buffer
	if code := runReleaseRowWith(context.Background(), args, &errout, load); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, &errout)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code := runReleaseRowWith(context.Background(), args, &errout, load); code != 1 {
		t.Fatalf("overwrite code=%d", code)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("output overwritten: %v", err)
	}
}

func TestReleaseRow_RecipeHashInputsPreserveHashBytes(t *testing.T) {
	recipe := retrieval.Recipe3()
	raw, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	row := map[string]json.RawMessage{
		"dense_recipe":   json.RawMessage(`{"document":{},"query":{}}`),
		"chunk_recipe":   json.RawMessage(`{"version": 1, "size": 10}`),
		"lexical_recipe": json.RawMessage(`{}`), "retrieval_recipe": raw,
	}
	doc, query := embedding.DocumentRecipe1(), embedding.QueryRecipe1()
	doc.Input, query.Input = "<document>&", "<query>&"
	row["dense_recipe"], err = json.Marshal(retrieval.DenseRecipe{Document: doc, Query: query})
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := releaseRecipeHashInputs(row)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(inputs["retrieval"]))
	if hex.EncodeToString(sum[:]) != recipe.Hash() || inputs["chunk"] != string(row["chunk_recipe"]) {
		t.Fatalf("hash inputs=%v", inputs)
	}
	for name, want := range map[string]string{"document": doc.Hash(), "query": query.Hash()} {
		sum = sha256.Sum256([]byte(inputs[name]))
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("%s hash input differs from the embedding recipe hash", name)
		}
	}
	row["dense_recipe"] = json.RawMessage(`[]`)
	if _, err = releaseRecipeHashInputs(row); err == nil {
		t.Fatal("malformed dense recipe accepted")
	}
}

func TestReleaseRowPool_ReadOnlyAndNoDatabaseFallback(t *testing.T) {
	cfg, err := releaseRowPoolConfig("postgres://synthetic@127.0.0.1/workshop", "corpus_vn_20990101_1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "corpus_vn_20990101_1" || cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] != "on" {
		t.Fatal("release metadata connection is not read-only")
	}
}
