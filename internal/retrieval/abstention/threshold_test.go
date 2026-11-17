package abstention

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestThreshold_FixedAnswersAndAbstentions(t *testing.T) {
	threshold, candidates := Threshold([]Score{
		{Value: .2, Negative: true}, {Value: .8},
		{Empty: true}, {Empty: true, Negative: true},
		{FixedAnswer: true}, {FixedAnswer: true, Negative: true},
	})
	require.Equal(t, .5, threshold)
	require.Equal(t, []Candidate{
		{Threshold: -1, TP: 1, FP: 1, TN: 2, FN: 2},
		{Threshold: .5, TP: 2, FP: 1, TN: 2, FN: 1},
		{Threshold: 2, TP: 2, FP: 2, TN: 1, FN: 1},
	}, candidates)
}

func TestThreshold_TiesAndAdjacentFloats(t *testing.T) {
	threshold, _ := Threshold([]Score{{Value: .2}, {Value: .8, Negative: true}})
	require.Equal(t, -1.0, threshold)
	upper := math.Nextafter(.5, 1)
	threshold, candidates := Threshold([]Score{{Value: .5, Negative: true}, {Value: upper}})
	require.Equal(t, upper, threshold)
	require.Len(t, candidates, 3)
}
