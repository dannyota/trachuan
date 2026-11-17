package build

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoldenCounts_PerVersionAdjudications(t *testing.T) {
	cases := []GoldenCase{
		{Code: "invented", Version: "v1", Adjudication: "accepted"},
		{Code: "invented", Version: "v2", Adjudication: "out_of_release"},
		{Code: "invented", Version: "v2", Adjudication: "out_of_release"},
		{Code: "invented", Version: "v1", Adjudication: "rejected"},
		{Code: "invented", Version: "v1", Adjudication: "unverified"},
	}
	require.Equal(t, []GoldenCaseCount{
		{Code: "invented", Version: "v1", Accepted: 1, Rejected: 1, Unverified: 1},
		{Code: "invented", Version: "v2", OutOfRelease: 2},
	}, CountGoldenCases(cases))
}
