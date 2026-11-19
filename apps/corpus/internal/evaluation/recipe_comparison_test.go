package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func withComparisonRecipe(t *testing.T, r Report, recipe retrieval.Recipe) Report {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["retrieval_recipe"], err = json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	r.Recipes["retrieval"] = recipe.Hash()
	return r
}

func TestMRRTolerance_RetrievalRecipeCalibration(t *testing.T) {
	for _, tc := range []struct {
		name        string
		threshold   float64
		calibration string
		pass        bool
	}{
		{"calibration alone", .53, "new", true},
		{"threshold alone", .54, "previous", false},
		{"threshold and calibration", .54, "new", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := retrieval.Recipe3()
			before.Abstention.Threshold = new(.53)
			before.Abstention.CalibrationSHA256 = "previous"
			after := before
			after.Abstention.Threshold = &tc.threshold
			after.Abstention.CalibrationSHA256 = tc.calibration
			p := withComparisonRecipe(t, toleranceReport(t, .7247), before)
			r := withComparisonRecipe(t, toleranceReport(t, .7226), after)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			for _, cohort := range []string{"old_product", "development"} {
				n := mrrRecord(t, r, cohort)
				if (n["status"] == "pass") != tc.pass {
					t.Fatalf("%s comparison: %v", cohort, n)
				}
			}
		})
	}
}

func TestMRRTolerance_CalibrationChangeKeepsWatermark(t *testing.T) {
	before := retrieval.Recipe3()
	before.Abstention.Threshold = new(.53)
	before.Abstention.CalibrationSHA256 = "first"
	p := withComparisonRecipe(t, toleranceReport(t, .7247), before)
	middle := withComparisonRecipe(t, toleranceReport(t, .7226), before)
	if err := finishQuality(&middle, &p); err != nil {
		t.Fatal(err)
	}
	after := before
	after.Abstention.CalibrationSHA256 = "second"
	r := withComparisonRecipe(t, toleranceReport(t, .719), after)
	if err := finishQuality(&r, &middle); err != nil {
		t.Fatal(err)
	}
	n := mrrRecord(t, r, "old_product")
	if n["high_water"] != .7247 || n["status"] != "fail" || n["tolerance"] != .005 {
		t.Fatalf("watermark: %v", n)
	}
}

func TestPreviousRecipe_LegacyReportMetadata(t *testing.T) {
	for _, format := range []string{"release", "manifest"} {
		t.Run(format, func(t *testing.T) {
			before := retrieval.Recipe3()
			before.Abstention.Threshold = new(.53)
			before.Abstention.CalibrationSHA256 = "old"
			after := before
			after.Abstention.CalibrationSHA256 = "new"
			previous := withComparisonRecipe(t, toleranceReport(t, .7247), before)
			previous.Version, previous.Scorer, previous.Country, previous.Rel = 1, "agent-eval/4", "vn", "20990101_1"
			previous.RetrievalRecipe = nil
			data, err := json.Marshal(previous)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			reportPath, metadataPath := filepath.Join(dir, "report.json"), filepath.Join(dir, "release.json")
			if err = os.WriteFile(reportPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			metadata := map[string]any{"country": "vn", "rel": "20990101_1"}
			if format == "release" {
				metadata["retrieval_recipe"] = before
			} else {
				metadata["recipes"] = map[string]any{"retrieval": before}
			}
			raw, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(metadataPath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			prior, hash, err := readPreviousReport(reportPath, metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if hash != hex.EncodeToString(sum[:]) {
				t.Fatal("metadata changed previous report hash")
			}
			current := withComparisonRecipe(t, toleranceReport(t, .7226), after)
			if err = finishQuality(&current, &prior); err != nil {
				t.Fatal(err)
			}
			if !sameMRRRecipes(&current, &prior) {
				t.Fatal("metadata recipe not comparable")
			}
			if n := mrrRecord(t, current, "old_product"); n["status"] != "fail" || n["tolerance"] != float64(0) {
				t.Fatalf("agent-eval/4 comparison not strict: %v", n)
			}
			before.Abstention.Threshold = new(.54)
			metadata["retrieval_recipe"] = before
			raw, err = json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(metadataPath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err = readPreviousReport(reportPath, metadataPath); err == nil {
				t.Fatal("metadata with wrong recipe hash accepted")
			}
		})
	}
}

func TestPreviousRecipe_FullObjectMustMatchHash(t *testing.T) {
	previous := toleranceReport(t, .7247)
	previous.Recipes["retrieval"] = "wrong"
	data, err := json.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = readPreviousReport(path, ""); err == nil {
		t.Fatal("previous recipe object with wrong hash accepted")
	}
}

func TestReport_CarriesFullRetrievalRecipe(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	engine := &fakeEngine{date: date}
	report, _, err := Run(context.Background(), engine, recordedSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	var recipe retrieval.Recipe
	if err = json.Unmarshal(report.RetrievalRecipe, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Hash() != report.Recipes["retrieval"] || recipe.Hash() != engine.Release().RetrievalRecipe.Hash() {
		t.Fatal("full recipe differs from hashed release recipe")
	}
}
