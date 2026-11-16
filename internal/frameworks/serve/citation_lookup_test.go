package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookup_ResolvesEveryCodeAndAliasSplit(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		ids         []int64
		want        []int64
	}{
		{"code", "FW-ALPHA A-1", []int64{10, 20}, []int64{10, 20}},
		{"alias after code prefix misses", "fw-alpha standard A-1", []int64{10, 20}, []int64{10, 20}},
		{"exact code before alias", "fw-alpha standard A-1", []int64{10, 20, 30}, []int64{30}},
		{"filtered code hit does not hide alias", "fw-alpha standard A-1", []int64{20}, []int64{20}},
		{"longer alias after shorter alias misses", "alpha standard A-1", []int64{10, 20, 40}, []int64{40}},
		{"unknown citation falls through", "fw-alpha standard A-99", []int64{10, 20}, nil},
		{"filtered version has no lookup", "fw-alpha A-1", []int64{40}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine()
			e.versions[0].aliases = []string{"fw-alpha standard", "Alpha"}
			e.versions[2].aliases = []string{"Alpha Standard"}
			all := map[int64]*control{
				10: {id: 10, versionID: 1, citation: "A-1", norm: "A-1"},
				20: {id: 20, versionID: 2, citation: "A-1", norm: "A-1"},
				30: {id: 30, versionID: 1, citation: "Standard A-1", norm: "STANDARDA-1"},
				40: {id: 40, versionID: 3, citation: "A-1", norm: "A-1"},
			}
			filtered := map[int64]*control{}
			for _, id := range tc.ids {
				filtered[id] = all[id]
			}
			var got []int64
			for _, hit := range e.lookup(tc.query, filtered) {
				got = append(got, hit.controlID)
			}
			require.Equal(t, tc.want, got)
		})
	}
}
