package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/corpusdb"
)

// BuildRecord describes the frozen corpus database and its dump.
type BuildRecord struct {
	Version                int              `json:"version"`
	Country                string           `json:"country"`
	Rel                    string           `json:"rel"`
	DatabaseName           string           `json:"database_name"`
	SchemaVersion          int              `json:"schema_version"`
	BuildInputHash         string           `json:"build_input_hash"`
	BuildRecipeHash        string           `json:"build_recipe_hash"`
	SourceManifestHash     string           `json:"source_manifest_hash"`
	IdentityInventoryHash  string           `json:"identity_inventory_hash"`
	ProvenanceSHA256       string           `json:"provenance_sha256,omitempty"`
	CorrectionsSHA256      string           `json:"corrections_sha256,omitempty"`
	IdentityMapSHA256      string           `json:"identity_map_sha256"`
	DevelopmentCasesSHA256 *string          `json:"development_cases_sha256"`
	DenseRecipe            json.RawMessage  `json:"dense_recipe"`
	ChunkRecipe            json.RawMessage  `json:"chunk_recipe"`
	LexicalRecipe          json.RawMessage  `json:"lexical_recipe"`
	RetrievalRecipe        json.RawMessage  `json:"retrieval_recipe"`
	LexicalStats           json.RawMessage  `json:"lexical_stats"`
	TaxonomyVersion        string           `json:"taxonomy_version"`
	Counts                 map[string]int64 `json:"counts"`
	Dump                   struct {
		SHA256 string `json:"sha256"`
		Bytes  int64  `json:"bytes"`
		Format string `json:"format"`
	} `json:"dump"`
	PostgresVersion string           `json:"postgres_version"`
	PGVectorVersion string           `json:"pgvector_version"`
	LtreeVersion    string           `json:"ltree_version"`
	PgDumpVersion   string           `json:"pg_dump_version"`
	WorkerVersion   string           `json:"worker_version"`
	DurationsMS     map[string]int64 `json:"durations_ms"`
	RelationBytes   int64            `json:"relation_bytes"`
	IndexBytes      int64            `json:"index_bytes"`
	BuiltAt         time.Time        `json:"built_at"`
}

func verifyBuildArtifact(store artifact.Store, art artifact.Result, run stage.Run, cfg Config) (BuildRecord, string, error) {
	path, hash := art.Path, art.SHA256
	dir, err := artifactDir(store, InventoryItem{ItemKey: run.Country + "/" + run.Rel, Path: &path, ArtifactSHA256: &hash})
	if err != nil {
		return BuildRecord{}, "", err
	}
	read := func(name string, out any) error {
		data, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			return fmt.Errorf("read %s: %w", name, e)
		}
		if e = json.Unmarshal(data, out); e != nil {
			return fmt.Errorf("decode %s: %w", name, e)
		}
		return nil
	}
	var record BuildRecord
	if err = read("build.json", &record); err != nil {
		return record, "", err
	}
	if record.DevelopmentCasesSHA256 == nil {
		return record, "", fmt.Errorf("build predates development fixture binding and must be rebuilt")
	}
	if err = verifyValidityReview(dir); err != nil {
		return record, "", err
	}
	var inventory Inventory
	if err = read("inputs.json", &inventory); err != nil {
		return record, "", err
	}
	final, err := DatabaseName(run.Country, run.Rel)
	if err != nil {
		return record, "", err
	}
	if record.Version != 1 || record.Country != run.Country || record.Rel != run.Rel || record.DatabaseName != final || record.SchemaVersion != corpusdb.SchemaVersion || inventory.Country != run.Country || inventory.SourceListHash != run.Snapshot.SourceListHash || record.BuildInputHash != art.InputHash || record.BuildInputHash != recordInputHash(run.Country, run.Rel, inventory, record.IdentityMapSHA256, record.ProvenanceSHA256, record.CorrectionsSHA256, *record.DevelopmentCasesSHA256) || *record.DevelopmentCasesSHA256 != cfg.Suite.Hashes.Development || record.BuildRecipeHash != New(cfg).Recipe(run.Snapshot) || record.SourceManifestHash != inventory.SourceManifestHash() || record.IdentityInventoryHash != inventory.IdentityHash() || record.Dump.Format != "custom" || record.PostgresVersion != cfg.Versions.PostgreSQLMajor || record.PGVectorVersion != cfg.Versions.PGVector || record.LtreeVersion != cfg.Versions.Ltree || record.PgDumpVersion != cfg.Versions.PgDump || record.WorkerVersion != run.Snapshot.WorkerVersion || record.BuiltAt.IsZero() {
		return record, "", fmt.Errorf("build artifact identity mismatch")
	}
	if inventory.Version == 2 {
		data, e := os.ReadFile(filepath.Join(dir, "provenance.json"))
		if e != nil {
			return record, "", fmt.Errorf("read provenance: %w", e)
		}
		if hashBytes(data) != record.ProvenanceSHA256 {
			return record, "", fmt.Errorf("provenance hash mismatch")
		}
	}
	calibration, err := os.ReadFile(filepath.Join(dir, "calibration.json"))
	if err != nil {
		return record, "", fmt.Errorf("read calibration: %w", err)
	}
	if err = verifyCalibration(record.RetrievalRecipe, calibration); err != nil {
		return record, "", err
	}

	dump := filepath.Join(dir, "corpus.dump")
	f, err := os.Open(dump)
	if err != nil {
		return record, "", fmt.Errorf("open dump: %w", err)
	}
	h := sha256.New()
	size, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return record, "", fmt.Errorf("hash dump: %w", copyErr)
	}
	if closeErr != nil {
		return record, "", fmt.Errorf("close dump: %w", closeErr)
	}
	if size != record.Dump.Bytes || fmt.Sprintf("%x", h.Sum(nil)) != record.Dump.SHA256 {
		return record, "", fmt.Errorf("dump hash or size mismatch")
	}
	return record, dump, nil
}

func verifyCalibration(recipeJSON, calibration []byte) error {
	var recipe struct {
		Abstention struct {
			Threshold         *float64 `json:"threshold"`
			CalibrationSHA256 string   `json:"calibration_sha256"`
		} `json:"abstention"`
	}
	var result struct {
		Threshold *float64 `json:"threshold"`
	}
	if err := json.Unmarshal(recipeJSON, &recipe); err != nil {
		return fmt.Errorf("decode retrieval recipe: %w", err)
	}
	if err := json.Unmarshal(calibration, &result); err != nil {
		return fmt.Errorf("decode calibration: %w", err)
	}
	if recipe.Abstention.CalibrationSHA256 != fmt.Sprintf("%x", sha256.Sum256(calibration)) || recipe.Abstention.Threshold == nil || result.Threshold == nil || *recipe.Abstention.Threshold != *result.Threshold {
		return fmt.Errorf("calibration hash or threshold mismatch")
	}
	return nil
}

func recordInputHash(country, rel string, inv Inventory, identityMap, provenanceHash, correctionsHash, developmentHash string) string {
	return withDevelopmentHash(buildHash(country, rel, inv, identityMap, provenanceHash, correctionInput{hash: correctionsHash, nonempty: correctionsHash != ""}), developmentHash)
}
