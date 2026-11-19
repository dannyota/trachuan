package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"danny.vn/trachuan/internal/retrieval"
)

func readPreviousReport(path, metadata string) (Report, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, "", fmt.Errorf("read previous report: %w", err)
	}
	previous, err := parsePreviousReport(data)
	if err != nil {
		return Report{}, "", err
	}
	if metadata != "" {
		if err = readPreviousRecipe(&previous, metadata); err != nil {
			return Report{}, "", err
		}
	}
	sum := sha256.Sum256(data)
	return previous, hex.EncodeToString(sum[:]), nil
}

func parsePreviousReport(data []byte) (Report, error) {
	previous, err := decodePreviousReport(data)
	if err != nil {
		return Report{}, err
	}
	if previous.Version >= 2 && len(previous.RetrievalRecipe) == 0 {
		return Report{}, fmt.Errorf("previous version %d retrieval recipe missing", previous.Version)
	}
	if len(previous.RetrievalRecipe) > 0 {
		var recipe retrieval.Recipe
		if err = json.Unmarshal(previous.RetrievalRecipe, &recipe); err != nil {
			return Report{}, fmt.Errorf("decode previous retrieval recipe: %w", err)
		}
		if recipe.Hash() != previous.Recipes["retrieval"] {
			return Report{}, fmt.Errorf("previous retrieval recipe hash differs from report")
		}
	}
	return previous, nil
}

func ParseDecisionReport(data []byte) (Report, error) {
	report, err := parsePreviousReport(data)
	if err != nil {
		return Report{}, err
	}
	if report.Version != 2 && report.Version != 3 {
		return Report{}, fmt.Errorf("previous report is version %d, want version 2 or 3", report.Version)
	}
	if report.Status != "complete" {
		return Report{}, fmt.Errorf("previous report status %q is not complete", report.Status)
	}
	return report, nil
}

type ComparedRelease struct {
	Country      string `json:"country"`
	Rel          string `json:"rel"`
	ReportSHA256 string `json:"report_sha256"`
}

func (r Report) ComparedRelease() (*ComparedRelease, error) {
	data, err := json.Marshal(r.PreviousDiff)
	if err != nil {
		return nil, fmt.Errorf("encode previous_diff: %v", err)
	}
	var diff struct {
		Previous *ComparedRelease `json:"previous"`
	}
	if err = json.Unmarshal(data, &diff); err != nil {
		return nil, fmt.Errorf("decode previous_diff: %v", err)
	}
	return diff.Previous, nil
}

func readPreviousRecipe(previous *Report, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read previous release metadata: %w", err)
	}
	var metadata struct {
		Country         string                     `json:"country"`
		Rel             string                     `json:"rel"`
		RetrievalRecipe json.RawMessage            `json:"retrieval_recipe"`
		Recipes         map[string]json.RawMessage `json:"recipes"`
	}
	if err = json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("decode previous release metadata: %w", err)
	}
	if metadata.Country != previous.Country || metadata.Rel != previous.Rel {
		return fmt.Errorf("previous release metadata identity differs from report")
	}
	raw := metadata.RetrievalRecipe
	if len(raw) == 0 {
		raw = metadata.Recipes["retrieval"]
	}
	var recipe retrieval.Recipe
	if err = json.Unmarshal(raw, &recipe); err != nil {
		return fmt.Errorf("decode previous retrieval recipe: %w", err)
	}
	if recipe.Hash() != previous.Recipes["retrieval"] {
		return fmt.Errorf("previous retrieval recipe hash differs from report")
	}
	encoded, err := recipe.JSON()
	if err != nil {
		return fmt.Errorf("encode previous retrieval recipe: %w", err)
	}
	var supplied, known, reported any
	if err = json.Unmarshal(raw, &supplied); err != nil {
		return fmt.Errorf("decode supplied retrieval recipe: %w", err)
	}
	if err = json.Unmarshal(encoded, &known); err != nil {
		return fmt.Errorf("decode known retrieval recipe: %w", err)
	}
	if !reflect.DeepEqual(supplied, known) {
		return fmt.Errorf("previous retrieval recipe has unknown or missing fields")
	}
	if len(previous.RetrievalRecipe) > 0 {
		if err = json.Unmarshal(previous.RetrievalRecipe, &reported); err != nil {
			return fmt.Errorf("decode reported retrieval recipe: %w", err)
		}
		if !reflect.DeepEqual(supplied, reported) {
			return fmt.Errorf("previous release recipe differs from report object")
		}
	}
	previous.RetrievalRecipe = raw
	return nil
}
