package build

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type floorMode struct {
	Recall *int `yaml:"recall_at_8" json:"recall_at_8"`
	MRR    *int `yaml:"mrr_at_8" json:"mrr_at_8"`
}

type floorValues struct {
	Open     *floorMode `yaml:"open" json:"open"`
	Filtered *floorMode `yaml:"filtered" json:"filtered"`
	Balanced *int       `yaml:"abstention_balanced_accuracy" json:"abstention_balanced_accuracy"`
}

type floorBaseline struct {
	Release  *string      `yaml:"release" json:"release"`
	Report   *string      `yaml:"report_sha256" json:"report_sha256"`
	Reviewer *string      `yaml:"reviewer" json:"reviewer"`
	Date     *string      `yaml:"date" json:"date"`
	Change   *string      `yaml:"cohort_change" json:"cohort_change"`
	Floors   *floorValues `yaml:"floors" json:"floors"`
}

func floorNumbers(f *floorValues) []*int {
	if f == nil || f.Open == nil || f.Filtered == nil {
		return nil
	}
	return []*int{f.Open.Recall, f.Open.MRR, f.Filtered.Recall, f.Filtered.MRR, f.Balanced}
}

func parseFloors(raw []byte) ([]floorBaseline, error) {
	invalid := func() ([]floorBaseline, error) { return nil, fmt.Errorf("invalid floor file") }
	var doc yaml.Node
	if yaml.Unmarshal(raw, &doc) != nil || !plainReviewYAML(&doc) || !wholeFloorScalars(&doc) {
		return invalid()
	}
	var file struct {
		Baselines *[]floorBaseline `yaml:"baselines"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if dec.Decode(&file) != nil || dec.Decode(new(any)) != io.EOF || file.Baselines == nil {
		return invalid()
	}
	var previous []*int
	for _, b := range *file.Baselines {
		if b.Release == nil || b.Report == nil || b.Reviewer == nil || b.Date == nil || b.Change == nil {
			return invalid()
		}
		if !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(*b.Release) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(*b.Report) || strings.TrimSpace(*b.Reviewer) == "" || strings.ContainsAny(*b.Change, "\r\n") {
			return invalid()
		}
		d, err := time.Parse("2006-01-02", *b.Date)
		if err != nil || d.Format("2006-01-02") != *b.Date {
			return invalid()
		}
		numbers := floorNumbers(b.Floors)
		if len(numbers) != 5 {
			return invalid()
		}
		for i, n := range numbers {
			if n == nil || *n < 0 || *n > 100 {
				return invalid()
			}
			if previous != nil && *n < *previous[i] && strings.TrimSpace(*b.Change) == "" {
				return nil, fmt.Errorf("floor lowered without cohort_change")
			}
		}
		previous = numbers
	}
	return *file.Baselines, nil
}

type FloorCheck struct {
	Metric  string   `json:"metric"`
	Percent int      `json:"percent"`
	Actual  *float64 `json:"actual"`
	Pass    bool     `json:"pass"`
}

type floorReference struct {
	Release      string `json:"release"`
	ReportSHA256 string `json:"report_sha256"`
}

type FloorResult struct {
	Status   string          `json:"status"`
	SHA256   string          `json:"sha256"`
	Baseline *floorReference `json:"baseline"`
	Checks   []FloorCheck    `json:"checks"`
	Pass     bool            `json:"pass"`
}

func checkFloors(baselines []floorBaseline, hash string, c CohortMetrics) FloorResult {
	r := FloorResult{Status: "no_baseline", SHA256: hash, Checks: []FloorCheck{}}
	if len(baselines) == 0 {
		return r
	}
	b := baselines[len(baselines)-1]
	r.Baseline = &floorReference{Release: *b.Release, ReportSHA256: *b.Report}
	r.Status = "evaluated"
	r.Pass = true
	metrics := []Metric{c.Modes["open"].Recall, c.Modes["open"].MRR, c.Modes["filtered"].Recall, c.Modes["filtered"].MRR, c.Modes["open"].BalancedAccuracy}
	names := []string{"open.recall_at_8", "open.mrr_at_8", "filtered.recall_at_8", "filtered.mrr_at_8", "abstention_balanced_accuracy"}
	for i, floor := range floorNumbers(b.Floors) {
		m := metrics[i]
		pass := m.Status == "available" && m.Value != nil && *m.Value >= float64(*floor)/100
		r.Checks = append(r.Checks, FloorCheck{names[i], *floor, m.Value, pass})
		r.Pass = r.Pass && pass
	}
	return r
}

func wholeFloorScalars(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Value == "recall_at_8" || key.Value == "mrr_at_8" || key.Value == "abstention_balanced_accuracy" {
				if value.Tag != "!!int" || !regexp.MustCompile(`^(0|[1-9][0-9]*)$`).MatchString(value.Value) {
					return false
				}
			}
		}
	}
	for _, child := range node.Content {
		if !wholeFloorScalars(child) {
			return false
		}
	}
	return true
}
