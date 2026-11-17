package build

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"github.com/jackc/pgx/v5"
)

func persistCalibration(ctx context.Context, db schema.DB, dir string, record CalibrationRecord) (string, map[string]any, error) {
	if err := writeJSON(dir, "calibration.json", record); err != nil {
		return "", nil, err
	}
	raw, err := readFile(filepath.Join(dir, "calibration.json"))
	if err != nil {
		return "", nil, err
	}
	hash := digest(raw)
	r := measuredRetrievalRecipe(record.Threshold, hash)
	if _, err = db.Exec(ctx, `UPDATE release SET retrieval_recipe=$1`, r); err != nil {
		return "", nil, hide("store measured retrieval recipe", err)
	}
	return hash, r, nil
}

func calibrateBuild(ctx context.Context, conn *pgx.Conn, dir string, d databaseInputs, opener CalibrationOpener) (string, map[string]any, error) {
	if d.caseHash == "" {
		return "", bootstrapRetrievalRecipe(), nil
	}
	if opener == nil {
		return "", nil, fmt.Errorf("calibration engine opener unavailable")
	}
	engine, closeEngine, err := opener(ctx, conn)
	if err != nil {
		return "", nil, hide("open calibration engine", err)
	}
	if closeEngine != nil {
		defer closeEngine()
	}
	r, err := CalibrateCases(ctx, engine, d.cases, d.caseHash)
	if err != nil {
		return "", nil, err
	}
	return persistCalibration(ctx, conn, dir, r)
}

func validateCalibration(dir string, m buildManifest) error {
	expected := bootstrapRetrievalRecipe()
	if m.CalibrationSHA256 != "" {
		raw, err := readFile(filepath.Join(dir, "calibration.json"))
		if err != nil {
			return err
		}
		var r CalibrationRecord
		if digest(raw) != m.CalibrationSHA256 || json.Unmarshal(raw, &r) != nil || r.Version != 1 {
			return fmt.Errorf("calibration record mismatch")
		}
		expected = measuredRetrievalRecipe(r.Threshold, m.CalibrationSHA256)
	}
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(m.RetrievalRecipe)
	if !equalJSON(a, b) {
		return fmt.Errorf("build retrieval recipe mismatch")
	}
	return nil
}
