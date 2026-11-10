package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk/normalizer"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
)

type buildRecipe struct {
	SchemaVersion       int    `json:"schema_version"`
	SchemaSHA256        string `json:"schema_sha256"`
	ChunkRecipeHash     string `json:"chunk_recipe_hash"`
	DocumentRecipeHash  string `json:"document_recipe_hash"`
	QueryRecipeHash     string `json:"query_recipe_hash"`
	LexicalRecipeHash   string `json:"lexical_recipe_hash"`
	RetrievalPolicyHash string `json:"retrieval_policy_hash"`
	NormalizerVersion   string `json:"normalizer_version"`
	StatusCodesSHA256   string `json:"status_codes_sha256"`
	CalibrationPolicy   string `json:"calibration_policy"`
	LegacyCasesSHA256   string `json:"legacy_cases_sha256"`
	PostgreSQLMajor     string `json:"postgresql_major"`
	PGVectorVersion     string `json:"pgvector_version"`
	LtreeVersion        string `json:"ltree_version"`
	PgDumpVersion       string `json:"pg_dump_version"`
	PgRestoreVersion    string `json:"pg_restore_version"`
}

func (r buildRecipe) hash() string {
	b, _ := json.Marshal(r)
	return stage.RecipeHash("build", "corpus/1", string(b))
}

// Versions identifies the database and command versions pinned to a build.
type Versions struct {
	PostgreSQLMajor, PGVector, Ltree, PgDump, PgRestore string
}

// Name returns the stage name.
func (*Stage) Name() string { return "build" }

// Recipe returns the build recipe hash for the frozen snapshot.
func (s *Stage) Recipe(snapshot stage.Snapshot) string {
	sources, err := source.Parse(snapshot.SourceList)
	if err != nil {
		return ""
	}
	status, err := json.Marshal(sources.StatusCodes)
	if err != nil {
		return ""
	}
	schema, err := corpusdb.SchemaSHA256(embedding.DocumentRecipe1().Dimensions)
	if err != nil {
		return ""
	}
	r := buildRecipe{SchemaVersion: corpusdb.SchemaVersion, SchemaSHA256: schema,
		ChunkRecipeHash: chunk.Recipe1().Hash(), DocumentRecipeHash: embedding.DocumentRecipe1().Hash(), QueryRecipeHash: embedding.QueryRecipe1().Hash(), LexicalRecipeHash: lexical.Recipe1().Hash(), RetrievalPolicyHash: retrieval.Recipe1().Hash(), NormalizerVersion: normalizer.Version, StatusCodesSHA256: fmt.Sprintf("%x", sha256.Sum256(status)), CalibrationPolicy: "provisional-old-product/1", LegacyCasesSHA256: s.cfg.Suite.Hashes.Legacy, PostgreSQLMajor: s.cfg.Versions.PostgreSQLMajor, PGVectorVersion: s.cfg.Versions.PGVector, LtreeVersion: s.cfg.Versions.Ltree, PgDumpVersion: s.cfg.Versions.PgDump, PgRestoreVersion: s.cfg.Versions.PgRestore}
	return r.hash()
}
