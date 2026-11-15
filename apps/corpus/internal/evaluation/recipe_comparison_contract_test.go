package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestRecipeComparison_PythonMatchesEvaluator(t *testing.T) {
	type comparison struct {
		Previous, Current Report
		Want              bool
	}
	var pairs []comparison
	for _, kind := range []string{"equal", "calibration", "threshold", "cap", "dense", "unknown", "boolean type", "legacy same hash", "legacy missing hash"} {
		before := retrieval.Recipe3()
		before.Abstention.Threshold = new(.53)
		before.Abstention.CalibrationSHA256 = "old"
		after := before
		switch kind {
		case "calibration":
			after.Abstention.CalibrationSHA256 = "new"
		case "threshold":
			after.Abstention.Threshold = new(.54)
		case "cap":
			after.DocumentCap = 6
		}
		previous := withComparisonRecipe(t, toleranceReport(t, .7247), before)
		current := withComparisonRecipe(t, toleranceReport(t, .7226), after)
		if kind == "dense" {
			current.Recipes["document"] = "new"
		}
		if kind == "unknown" || kind == "boolean type" {
			var raw map[string]any
			if err := json.Unmarshal(current.RetrievalRecipe, &raw); err != nil {
				t.Fatal(err)
			}
			if kind == "unknown" {
				raw["unknown"] = true
			} else {
				raw["default_in_force_only"] = 1
			}
			data, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			current.RetrievalRecipe = data
		}
		if kind == "legacy same hash" || kind == "legacy missing hash" {
			previous.Version = 1
			previous.RetrievalRecipe = nil
			if kind == "legacy missing hash" {
				previous.Recipes["retrieval"] = ""
				current.Recipes["retrieval"] = ""
			}
		}
		want := kind == "equal" || kind == "calibration" || kind == "legacy same hash"
		if got := sameMRRRecipes(&current, &previous); got != want {
			t.Fatalf("Go %s: %v, want %v", kind, got, want)
		}
		pairs = append(pairs, comparison{previous, current, want})
	}
	data, err := json.Marshal(pairs)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), "python3", "-c", `import json,sys; from corpus_comparison import same_recipes; print(json.dumps([same_recipes(p['Current'],p['Previous']) for p in json.load(sys.stdin)]))`)
	command.Dir = filepath.Join("..", "..", "..", "..", "deploy", "github")
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	command.Stdin = bytes.NewReader(data)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Python comparison: %v: %s", err, output)
	}
	var results []bool
	if err = json.Unmarshal(output, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != len(pairs) {
		t.Fatalf("Python comparison count: %d", len(results))
	}
	for i, pair := range pairs {
		if results[i] != pair.Want {
			t.Fatalf("Python comparison %d: %v, want %v", i, results[i], pair.Want)
		}
	}
}
