package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type EvaluationReport struct {
	Version             int               `json:"version"`
	Release             string            `json:"rel"`
	DumpSHA256          string            `json:"dump_sha256"`
	BuildManifestSHA256 string            `json:"build_manifest_sha256"`
	RecipeHashes        map[string]string `json:"recipe_hashes"`
	CaseFileSHA256      string            `json:"case_file_sha256"`
	ReviewSHA256        string            `json:"review_sha256"`
	CaseCounts          []GoldenCaseCount `json:"case_counts"`
	CalibrationSHA256   string            `json:"calibration_sha256"`
	Threshold           *float64          `json:"threshold"`
	Noise               string            `json:"noise"`
	EvaluationResult
	Floors       map[string]FloorResult `json:"floors"`
	Previous     *previousReference     `json:"previous"`
	PreviousDiff previousComparison     `json:"previous_diff"`
	QualityPass  bool                   `json:"quality_pass"`
}

func optionalArtifact(path string) ([]byte, error) {
	raw, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return raw, err
}
func RecipeSHA256(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var normalized any
	if json.Unmarshal(raw, &normalized) != nil {
		return ""
	}
	raw, _ = json.Marshal(normalized)
	return digest(raw)
}

func evaluateStep(ctx context.Context, base, dir string, request DatabaseRequest, database Database, publish func() error) error {
	evaluator, ok := database.(Evaluator)
	if !ok {
		return fmt.Errorf("database evaluator unavailable")
	}
	if err := validateBuild(filepath.Join(base, "build")); err != nil {
		return err
	}
	data, err := readDatabaseInputs(base)
	if err != nil {
		return err
	}
	raw, err := readFile(filepath.Join(base, "build/build.json"))
	if err != nil {
		return err
	}
	var built buildManifest
	if err = json.Unmarshal(raw, &built); err != nil {
		return hide("decode evaluation build manifest", err)
	}
	builtCases, err := loadRows[GoldenCase](filepath.Join(base, "build/cases.jsonl"))
	if err != nil {
		return err
	}
	if RecipeSHA256(builtCases) != RecipeSHA256(data.cases) {
		return fmt.Errorf("built case mapping mismatch")
	}
	request.DumpPath = filepath.Join(base, "build/frameworks.dump")
	req := EvaluationRequest{DatabaseRequest: request, Cases: data.cases, CaseFileSHA256: data.caseHash, Controls: data.controls, CalibrationSHA256: built.CalibrationSHA256, RecipeHashes: map[string]string{}, InputHashes: map[string]string{}, RetrievalRecipe: built.RetrievalRecipe}
	if built.CalibrationSHA256 != "" {
		calibrationRaw, readErr := readFile(filepath.Join(base, "build/calibration.json"))
		if readErr != nil {
			return readErr
		}
		var record CalibrationRecord
		if err = json.Unmarshal(calibrationRaw, &record); err != nil {
			return hide("decode evaluation calibration", err)
		}
		req.Calibration = &record
	}
	for name, step := range data.steps {
		req.RecipeHashes[name] = RecipeSHA256(step.Recipe)
		req.InputHashes[name] = step.InputHash
	}
	recipes := map[string]string{}
	for name, hash := range req.RecipeHashes {
		recipes[name] = hash
	}
	recipes["build"] = RecipeSHA256(built.Recipe)
	recipes["evaluate"] = RecipeSHA256(stepRecipe("evaluate"))
	review, err := optionalArtifact(filepath.Join(base, "snapshot/trachuan/golden-review.yaml"))
	if err != nil {
		return err
	}
	report := EvaluationReport{Version: 1, Release: request.Rel, DumpSHA256: built.Dump.SHA256, BuildManifestSHA256: digest(raw), RecipeHashes: recipes, CaseFileSHA256: data.caseHash, CaseCounts: CountGoldenCases(data.cases), CalibrationSHA256: built.CalibrationSHA256, Noise: "Small cohorts: one case moves recall by several percentage points. Abstention is biased by calibration on the scored public cohort.", Floors: map[string]FloorResult{}}
	previous, previousData, previousErr := readPreviousReport(base, request.Rel)
	if previousErr != nil {
		return previousErr
	}
	report.Previous = previous
	if review != nil {
		report.ReviewSHA256 = digest(review)
	}
	if req.Calibration != nil {
		report.Threshold = &req.Calibration.Threshold
	}
	baselineFiles := map[string]string{"public": "sources/frameworks-floors.yaml", "licensed": "trachuan/floors.yaml"}
	baselines := map[string][]floorBaseline{}
	floorHashes := map[string]string{}
	for _, cohort := range []string{"public", "licensed"} {
		file, fileErr := optionalArtifact(filepath.Join(base, "snapshot", baselineFiles[cohort]))
		if fileErr != nil {
			return fileErr
		}
		if file != nil {
			baselines[cohort], err = parseFloors(file)
			if err != nil {
				return err
			}
			floorHashes[cohort] = digest(file)
		}
	}
	err = evaluator.Evaluate(ctx, req, func(result EvaluationResult) error {
		report.EvaluationResult = result
		for _, cohort := range []string{"public", "licensed"} {
			report.Floors[cohort] = checkFloors(baselines[cohort], floorHashes[cohort], result.Cohorts[cohort])
		}
		report.PreviousDiff = comparePrevious(previous, previousData, result.Cases, report.CaseCounts)
		report.QualityPass = qualityPass(report)
		if writeErr := writeJSON(dir, "report.json", report); writeErr != nil {
			return writeErr
		}
		return publish()
	})
	if err != nil {
		return hide("evaluate database", err)
	}
	return nil
}

func qualityPass(r EvaluationReport) bool {
	if r.Previous == nil {
		if r.PreviousDiff.Status != "not_applicable" {
			return false
		}
	} else if r.PreviousDiff.Status != "evaluated" || !r.PreviousDiff.Pass {
		return false
	}
	if !r.Restored || r.RetrievalRecipeStatus != "measured" || r.CalibrationSHA256 == "" || r.Threshold == nil || !r.Invariant.Pass {
		return false
	}
	for _, g := range r.Gaps {
		if g.Blocks && g.Count > 0 {
			return false
		}
	}
	for _, cohort := range []string{"public", "licensed"} {
		c, ok := r.Cohorts[cohort]
		if !ok || !c.Filter.Pass || !r.Floors[cohort].Pass {
			return false
		}
	}
	return true
}
