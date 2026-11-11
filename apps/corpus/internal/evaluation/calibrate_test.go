package evaluation

import (
	"context"
	"math"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestCalibrationThreshold_SeparatesAndBreaksTies(t *testing.T) {
	tests := []struct {
		name   string
		scores []calibrationScore
		want   float64
	}{
		{"separate", []calibrationScore{{value: .2, negative: true}, {value: .6}, {value: .8}}, .4},
		{"no positives", []calibrationScore{{value: .8, negative: true}, {value: .2}}, 2},
		{"no negatives", []calibrationScore{{value: .6}}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := calibrationThreshold(tt.scores)
			if got != tt.want {
				t.Fatalf("threshold = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalibrationThreshold_RoundedMidpointUsesUpper(t *testing.T) {
	lower := .5
	upper := math.Nextafter(lower, 1)
	got, _ := calibrationThreshold([]calibrationScore{{value: lower, negative: true}, {value: upper}})
	if got != upper {
		t.Fatalf("got %.17g, want %.17g", got, upper)
	}
}

type cohortCalibrationEngine struct {
	fakeEngine
	oldNegative, developmentNegative float64
}

func (e *cohortCalibrationEngine) Collect(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	out := retrieval.Arms{Validation: p.Validation, Depths: d, Date: e.date}
	cosine := .9
	switch p.Request.Query {
	case "Câu hỏi không":
		cosine = e.oldNegative
	case "Phủ định phát triển":
		cosine = e.developmentNegative
	}
	hit := lossCandidate(1, "doc", "one")
	hit.Cosine = cosine
	out.Primary.Dense = []retrieval.Candidate{hit}
	out.Primary.Lexical = []retrieval.Candidate{hit}
	out.Primary.Members = []retrieval.Candidate{hit}
	return out, nil
}

func TestCalibrate_UsesOldProductAndDevelopment(t *testing.T) {
	for _, oldHigher := range []bool{false, true} {
		s := developmentSuite()
		s.Development[1].Question = "Phủ định phát triển"
		e := &cohortCalibrationEngine{oldNegative: .2, developmentNegative: .7}
		if oldHigher {
			e.oldNegative, e.developmentNegative = .7, .2
		}
		cal, err := Calibrate(context.Background(), e, s)
		if err != nil {
			t.Fatal(err)
		}
		if cal.Status != "old-product+development" || len(cal.Scores) != 4 || cal.Threshold != .8 || !cal.Success {
			t.Fatalf("calibration: %+v", cal)
		}
		for _, candidate := range cal.Candidates {
			if candidate.Threshold == cal.Threshold && (candidate.TP != 2 || candidate.TN != 2 || candidate.FP != 0 || candidate.FN != 0) {
				t.Fatal("selected threshold does not separate both cohorts")
			}
		}
		for _, score := range cal.Scores {
			if strings.HasPrefix(score.CaseID, "reject") {
				t.Fatal("rejection check fitted")
			}
		}
	}
	cal, err := Calibrate(context.Background(), &cohortCalibrationEngine{oldNegative: .2}, recordedSuite())
	if err != nil {
		t.Fatal(err)
	}
	if cal.Status != "provisional-old-product" || len(cal.Scores) != 2 {
		t.Fatal("empty development changed provisional calibration")
	}
}
