package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

type DatabaseRequest struct {
	WorkDir, Rel, RunID, DumpPath string
	Runtime                       map[string]string
}

type Database interface {
	Build(context.Context, DatabaseRequest, func(context.Context, *pgx.Conn) error, func() error) error
}

type Vector struct {
	Ordinal     int
	TokenCount  int
	InputSHA256 string
	Values      []float32
}

type buildManifest struct {
	Dump struct {
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	} `json:"dump"`
	Recipe            recipe            `json:"recipe"`
	RetrievalRecipe   map[string]any    `json:"retrieval_recipe"`
	CalibrationSHA256 string            `json:"calibration_sha256,omitempty"`
	CaseCounts        []GoldenCaseCount `json:"case_counts"`
}

func databaseRecipe() map[string]any {
	hash, _ := schema.SchemaSHA256(embedding.DocumentRecipe1().Dimensions)
	return map[string]any{
		"schema_version": schema.SchemaVersion, "schema_sha256": hash,
		"dense_recipe": denseRecipe(), "lexical_recipe": lexical.Recipe1(),
		"gap_kinds": gapKinds(), "loader_version": 1,
		"lexical_runtime":  lexicalRuntime(),
		"retrieval_recipe": bootstrapRetrievalRecipe(), "calibration": "public-free-only-open/1",
		"query_recipe_hash": embedding.QueryRecipe1().Hash(),
	}
}

func denseRecipe() retrieval.DenseRecipe {
	document, query := embedding.DocumentRecipe1(), embedding.QueryRecipe1()
	return retrieval.DenseRecipe{Version: 1, Model: document.Model, Dimensions: document.Dimensions, Document: document, DocumentHash: document.Hash(), Query: query, QueryHash: query.Hash()}
}

func buildStep(ctx context.Context, base, dir string, request DatabaseRequest, embedder Embedder, database Database, opener CalibrationOpener, publish func() error) error {
	if database == nil {
		return fmt.Errorf("database builder unavailable")
	}
	data, err := readDatabaseInputs(base)
	if err != nil {
		return err
	}
	request.DumpPath = filepath.Join(dir, "frameworks.dump")
	request.Runtime = lexicalRuntime()
	if err = writeRows(dir, "cases.jsonl", data.cases); err != nil {
		return err
	}
	calibrationHash, retrievalRecipe := "", bootstrapRetrievalRecipe()
	err = database.Build(ctx, request, func(ctx context.Context, conn *pgx.Conn) error {
		if loadErr := loadDatabase(ctx, conn, base, request.Rel, data, embedder); loadErr != nil {
			return loadErr
		}
		var calibrationErr error
		calibrationHash, retrievalRecipe, calibrationErr = calibrateBuild(ctx, conn, dir, data, opener)
		return calibrationErr
	}, func() error {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		info, statErr := os.Lstat(request.DumpPath)
		if statErr != nil {
			return hide("inspect database dump", statErr)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("invalid database dump")
		}
		if err = os.Chmod(request.DumpPath, 0600); err != nil {
			return hide("protect database dump", err)
		}
		dump, readErr := readFile(request.DumpPath)
		if readErr != nil {
			return readErr
		}
		if data.caseHash != "" && calibrationHash == "" {
			return fmt.Errorf("reviewed cases were not calibrated")
		}
		m := buildManifest{Recipe: stepRecipe("build"), RetrievalRecipe: retrievalRecipe, CalibrationSHA256: calibrationHash, CaseCounts: CountGoldenCases(data.cases)}
		m.Dump.SHA256, m.Dump.Size = digest(dump), int64(len(dump))
		if syncErr := syncDump(request.DumpPath); syncErr != nil {
			return syncErr
		}
		if err = writeJSON(dir, "build.json", m); err != nil {
			return err
		}
		return publish()
	})
	if err != nil {
		return hide("build database", err)
	}
	return nil
}

func syncDump(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return hide("open database dump", err)
	}
	defer func() { _ = f.Close() }()
	if err = f.Sync(); err != nil {
		return hide("sync database dump", err)
	}
	if err = f.Close(); err != nil {
		return hide("close database dump", err)
	}
	return nil
}

func validateBuild(dir string) error {
	data, err := readFile(filepath.Join(dir, "build.json"))
	if err != nil {
		return err
	}
	var m buildManifest
	if err = json.Unmarshal(data, &m); err != nil {
		return hide("decode build manifest", err)
	}
	dump, err := readFile(filepath.Join(dir, "frameworks.dump"))
	if err != nil {
		return err
	}
	expected, _ := json.Marshal(stepRecipe("build"))
	actual, _ := json.Marshal(m.Recipe)
	if !equalJSON(expected, actual) || m.Dump.Size != int64(len(dump)) || len(dump) == 0 || m.Dump.SHA256 != digest(dump) {
		return fmt.Errorf("build manifest mismatch")
	}
	return validateCalibration(dir, m)
}
