package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

type releaseComparison struct {
	Mode   retrieval.Mode    `json:"mode"`
	Arms   retrieval.ArmMode `json:"arms"`
	Limit  int               `json:"limit"`
	Name   string            `json:"name"`
	Gating bool              `json:"gating"`
}

func releaseComparisons(records []NonRegression) []releaseComparison {
	result := make([]releaseComparison, 0, len(records))
	for _, record := range records {
		result = append(result, releaseComparison{record.Mode, record.Arms, record.Limit, record.Name, record.Gating})
	}
	return result
}

func TestReleaseValidator_ContractMatchesEvaluationOutput(t *testing.T) {
	ctx := context.Background()
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	report, _, err := Run(ctx, &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "python3", "-c", "import json; from corpus_manifest import non_regression_contract; print(json.dumps(non_regression_contract()))")
	command.Dir = filepath.Join("..", "..", "..", "..", "deploy", "github")
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read Python release contract: %v: %s", err, data)
	}
	var contract map[string][]releaseComparison
	if err = json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	emitted := make(map[string][]releaseComparison, len(report.Cohorts))
	for name, cohort := range report.Cohorts {
		emitted[name] = releaseComparisons(cohort.NonRegression)
	}
	if !reflect.DeepEqual(emitted, contract) {
		t.Fatalf("release validator contract differs from evaluation: emitted=%v contract=%v", emitted, contract)
	}
	if !reflect.DeepEqual(releaseComparisons(report.NonRegression), contract["old_product"]) {
		t.Fatal("top-level comparisons differ from old_product")
	}
}
