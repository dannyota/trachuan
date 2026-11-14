package build

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk/normalizer"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
)

func TestBuildRecipe_EveryFieldAffectsHash(t *testing.T) {
	r := buildRecipe{}
	base := r.hash()
	v := reflect.ValueOf(&r).Elem()
	for i := 0; i < v.NumField(); i++ {
		x := v.Field(i)
		switch x.Kind() {
		case reflect.String:
			x.SetString("changed")
		case reflect.Int:
			x.SetInt(42)
		default:
			t.Fatalf("unhandled field %s", v.Type().Field(i).Name)
		}
		if r.hash() == base {
			t.Errorf("field %s did not change hash", v.Type().Field(i).Name)
		}
		x.Set(reflect.Zero(x.Type()))
	}
}

func TestPlan_RejectsYearZero(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("year zero reached workshop: %v", r)
		}
	}()
	_, err := New(Config{}).Plan(context.Background(), stage.Env{}, stage.Run{Country: "vn", Rel: "00000101"})
	if err == nil {
		t.Fatal("accepted year zero")
	}
}

func TestBuildInput_MapBytesChangeInputNotRecipe(t *testing.T) {
	first := buildInputHash("vn", "20990101", "manifest", "map-one")
	second := buildInputHash("vn", "20990101", "manifest", "map-two")
	if first == second {
		t.Fatal("map did not change input hash")
	}
	snapshot := stage.Snapshot{SourceList: []byte(`{"country":"vn","status_codes":{}}`)}
	a := New(Config{Suite: evaluation.Suite{MapBytes: []byte("map-one"), Hashes: evaluation.FixtureHashes{IdentityMap: "map-one"}}}).Recipe(snapshot)
	b := New(Config{Suite: evaluation.Suite{MapBytes: []byte("map-two"), Hashes: evaluation.FixtureHashes{IdentityMap: "map-two"}}}).Recipe(snapshot)
	if a == "" || a != b {
		t.Fatalf("map changed build recipe: %q != %q", a, b)
	}
}

func TestBuild_InitialEvaluationSummary(t *testing.T) {
	if got := initialEvaluationSummary(); string(got) != `{"status":"not_run","release_eligible":false}` {
		t.Fatalf("summary = %s", got)
	}
}

func TestRecipe_NumberNormalizationVersionChangesHash(t *testing.T) {
	schema, err := corpusdb.SchemaSHA256(embedding.DocumentRecipe1().Dimensions)
	if err != nil {
		t.Fatal(err)
	}
	r := buildRecipe{SchemaVersion: corpusdb.SchemaVersion, SchemaSHA256: schema,
		ChunkRecipeHash: chunk.Recipe2().Hash(), DocumentRecipeHash: embedding.DocumentRecipe1().Hash(), QueryRecipeHash: embedding.QueryRecipe1().Hash(), LexicalRecipeHash: lexical.Recipe1().Hash(), RetrievalPolicyHash: retrieval.Recipe3().Hash(), NormalizerVersion: normalizer.Version, NumberNormalizationVersion: retrieval.NumberNormalizationVersion, StatusCodesSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("null"))), ValidityRulesVersion: "validity/2", CalibrationPolicy: "provisional-old-product/balanced/1"}
	got := New(Config{}).Recipe(stage.Snapshot{SourceList: []byte(`{}`)})
	if got != r.hash() {
		t.Fatalf("Recipe() = %s, want %s", got, r.hash())
	}
	r.NumberNormalizationVersion = "different"
	if got == r.hash() {
		t.Fatal("number normalization version did not change hash")
	}
}

func TestBuildRecipe_CorrectionBytesDoNotChangeHash(t *testing.T) {
	first := stage.Snapshot{SourceList: []byte(`{"corrections_bytes":"[]\n"}`)}
	second := stage.Snapshot{SourceList: []byte(`{"corrections_bytes":"[]\n# reviewed\n"}`)}
	if New(Config{}).Recipe(first) != New(Config{}).Recipe(second) {
		t.Fatal("correction bytes changed build recipe")
	}
}

func TestRecipe_ValidityRulesVersion2(t *testing.T) {
	schema, err := corpusdb.SchemaSHA256(embedding.DocumentRecipe1().Dimensions)
	if err != nil {
		t.Fatal(err)
	}
	r := buildRecipe{SchemaVersion: corpusdb.SchemaVersion, SchemaSHA256: schema,
		ChunkRecipeHash: chunk.Recipe2().Hash(), DocumentRecipeHash: embedding.DocumentRecipe1().Hash(), QueryRecipeHash: embedding.QueryRecipe1().Hash(), LexicalRecipeHash: lexical.Recipe1().Hash(), RetrievalPolicyHash: retrieval.Recipe3().Hash(), NormalizerVersion: normalizer.Version, NumberNormalizationVersion: retrieval.NumberNormalizationVersion, StatusCodesSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("null"))), ValidityRulesVersion: "validity/2", CalibrationPolicy: "provisional-old-product/balanced/1"}
	want := r.hash()
	if got := New(Config{}).Recipe(stage.Snapshot{SourceList: []byte(`{}`)}); got != want {
		t.Fatalf("recipe = %s, want %s", got, want)
	}
}

func TestBuildInput_CorrectionHash(t *testing.T) {
	legacy := Inventory{}
	modern := Inventory{Version: 2}
	empty := hashBytes([]byte("[]"))
	if got := inventoryInputHash("vn", "20990101", legacy, "map", correctionInput{"", false}); got != buildInputHash("vn", "20990101", legacy.SourceManifestHash(), "map") {
		t.Fatal("empty legacy corrections changed hash")
	}
	if inventoryInputHash("vn", "20990101", legacy, "map", correctionInput{"one", true}) == inventoryInputHash("vn", "20990101", legacy, "map", correctionInput{"two", true}) {
		t.Fatal("legacy corrections not bound")
	}
	if inventoryInputHash("vn", "20990101", modern, "map", correctionInput{empty, false}) == inventoryInputHash("vn", "20990101", modern, "map", correctionInput{"changed", false}) {
		t.Fatal("modern corrections not bound")
	}
}

func TestBuildInput_RecordMatchesCorrectionsHash(t *testing.T) {
	for _, tc := range []struct {
		version  int
		hash     string
		nonempty bool
	}{{0, "", false}, {0, "nonempty", true}, {2, hashBytes([]byte("[]")), false}} {
		inv := Inventory{Version: tc.version}
		got := recordInputHash("vn", "20990101", inv, "map", hashJSON(inv.Provenance), tc.hash, "")
		want := withDevelopmentHash(inventoryInputHash("vn", "20990101", inv, "map", correctionInput{hash: tc.hash, nonempty: tc.nonempty}), "")
		if got != want {
			t.Fatalf("version %d: got %s want %s", tc.version, got, want)
		}
	}
}

func TestBuildRecipe_Version3(t *testing.T) {
	r := buildRecipe{}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.hash(), stage.RecipeHash("build", "corpus/3", string(b)); got != want {
		t.Fatalf("hash = %s, want %s", got, want)
	}
}
