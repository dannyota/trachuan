//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

const frameworksRecipeHashScript = `import json,sys
sys.path.insert(0,sys.argv[1])
from frameworks_recipes import decode_floats,recipe_sha256
from frameworks_release import check_recipes
row=decode_floats(open(sys.argv[2],'rb').read())
request=decode_floats(sys.stdin.buffer.read())
hashes={name:recipe_sha256(row[name]) for name in request['columns']}
try:
    check_recipes(request['report'],row,request['build'],request['evaluate'])
    refused=''
except ValueError as error:
    refused=str(error)
print(json.dumps({'hashes':hashes,'refused':refused}))
`

type frameworksRecipeVector struct {
	SHA256 string          `json:"sha256"`
	Input  json.RawMessage `json:"input"`
}

type frameworksRecipeRequest struct {
	Columns  []string                     `json:"columns"`
	Report   map[string]map[string]string `json:"report"`
	Build    map[string]json.RawMessage   `json:"build"`
	Evaluate map[string]json.RawMessage   `json:"evaluate"`
}

type frameworksRecipeResult struct {
	Hashes  map[string]string `json:"hashes"`
	Refused string            `json:"refused"`
}

func frameworksRecipeVectors(t *testing.T, root string) map[string]frameworksRecipeVector {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "deploy/github/testdata/frameworks-recipe-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			Name string `json:"name"`
			frameworksRecipeVector
		} `json:"vectors"`
	}
	if err = json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	vectors := map[string]frameworksRecipeVector{}
	for _, vector := range file.Vectors {
		vectors[vector.Name] = vector.frameworksRecipeVector
	}
	return vectors
}

func frameworksEdgeRecipe(t *testing.T, input json.RawMessage) string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	value["controls"] = "\b\f\n\r\t\x01\x1f\x7f"
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func frameworksPythonHashes(t *testing.T, root, path string, request frameworksRecipeRequest) frameworksRecipeResult {
	t.Helper()
	input, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", frameworksRecipeHashScript, filepath.Join(root, "deploy/github"), path)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("hash release row with Python: %v: %.400s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	var result frameworksRecipeResult
	if err = json.Unmarshal(output, &result); err != nil {
		t.Fatal("decode Python recipe hashes")
	}
	return result
}

func frameworksRecipeDatabase(t *testing.T) (*pgx.Conn, string, string) {
	t.Helper()
	ctx := t.Context()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal("open fixture administrator")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	reserved, key, err := testpg.CreateCorpus(ctx, admin, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	name := strings.Replace(reserved, "corpus_vn_", "frameworks_", 1)
	if !testpg.DropCorpusDatabase(t, admin, reserved, key) {
		t.Fatal("release fixture marker mismatch")
	}
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, key) })
	if _, err = admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '{\"run_key\":\""+key+"\"}'"); err != nil {
		t.Fatal(err)
	}
	builder := base.String()
	base.Path = "/" + name
	conn, err := pgx.Connect(ctx, base.String())
	if err != nil {
		t.Fatal("connect invented release")
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn, strings.TrimPrefix(name, "frameworks_"), builder
}

func TestFrameworksReleaseRow_RecipeHashesSurviveJSONB(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	vectors := frameworksRecipeVectors(t, root)
	ctx := t.Context()
	conn, rel, builder := frameworksRecipeDatabase(t)
	if _, err = conn.Exec(ctx, `CREATE EXTENSION vector`); err != nil {
		t.Fatal(err)
	}
	if err = schema.ApplyTables(ctx, conn, 1024); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `ALTER TABLE release ADD COLUMN edge_recipe jsonb NOT NULL, ADD COLUMN negative_zero_recipe jsonb NOT NULL`); err != nil {
		t.Fatal(err)
	}
	edge := frameworksEdgeRecipe(t, vectors["encoding_edges"].Input)
	hash := strings.Repeat("a", 64)
	if _, err = conn.Exec(ctx, `INSERT INTO release(
 singleton,kind,rel,schema_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,
 snapshot_recipe,parse_recipe,relate_recipe,embed_recipe,
 snapshot_input_hash,parse_input_hash,relate_input_hash,chunk_input_hash,embed_input_hash,
 framework_count,version_count,control_count,built_at,edge_recipe,negative_zero_recipe)
 VALUES(true,'frameworks',$1,1,'{}',$2::text::jsonb,'{}','{}',
 $3::text::jsonb,$4::text::jsonb,$5::text::jsonb,$6::text::jsonb,
 $7,$7,$7,$7,$7,0,0,0,now(),$8::text::jsonb,$9::text::jsonb)`,
		rel, string(vectors["step_chunk"].Input), string(vectors["step_snapshot"].Input), string(vectors["step_parse"].Input),
		string(vectors["step_relate"].Input), string(vectors["step_embed"].Input), hash, edge, string(vectors["negative_zero"].Input)); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	t.Setenv("FRAMEWORKS_WORK_DIR", work)
	t.Setenv("CORPUS_BUILDER_DATABASE_URL", builder)
	var out, stderr bytes.Buffer
	if code := Run(ctx, []string{"frameworks", "release-row", rel}, &out, &stderr); code != 0 {
		t.Fatalf("release-row failed: %s", stderr.String())
	}
	path := filepath.Join(work, rel, "release", "release.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err = json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"edge_recipe": build.RecipeSHA256(json.RawMessage(edge)), "negative_zero_recipe": build.RecipeSHA256(map[string]any{"value": 0})}
	report := map[string]string{"build": vectors["step_build"].SHA256, "evaluate": vectors["step_evaluate"].SHA256}
	for _, step := range []string{"snapshot", "parse", "relate", "chunk", "embed"} {
		expected[step+"_recipe"] = vectors["step_"+step].SHA256
		report[step] = vectors["step_"+step].SHA256
	}
	if expected["negative_zero_recipe"] == vectors["negative_zero"].SHA256 {
		t.Fatal("negative zero fixture does not distinguish the round trip")
	}
	request := frameworksRecipeRequest{Report: map[string]map[string]string{"recipe_hashes": report},
		Build:    map[string]json.RawMessage{"recipe": vectors["step_build"].Input},
		Evaluate: map[string]json.RawMessage{"recipe": vectors["step_evaluate"].Input}}
	for column, want := range expected {
		request.Columns = append(request.Columns, column)
		if got := build.RecipeSHA256(row[column]); got != want {
			t.Errorf("Go hash of %s after JSONB differs", column)
		}
	}
	result := frameworksPythonHashes(t, root, path, request)
	if result.Refused != "" {
		t.Fatalf("Python recipe check refused the row: %s", result.Refused)
	}
	for column, want := range expected {
		if result.Hashes[column] != want {
			t.Errorf("Python hash of %s after JSONB differs", column)
		}
	}
	report["parse"] = strings.Repeat("0", 64)
	if result = frameworksPythonHashes(t, root, path, request); !strings.Contains(result.Refused, "parse recipe differs from report") {
		t.Fatal("Python recipe check accepted a changed report hash")
	}
}
