package evaluation

import (
	"math"
	"testing"
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
