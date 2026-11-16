package relate

import (
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/stretchr/testify/require"
)

func TestBuild_DropsInvalidEndsBeforeEvidenceAndDuplicates(t *testing.T) {
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1", Access: "free"}, {Code: "beta", Version: "1", Access: "licensed"}}}
	for _, tc := range []struct {
		name     string
		from, to Key
	}{
		{"unregistered framework", Key{"alpha", "1", "A"}, Key{"unregistered", "1", "B"}},
		{"invalid version", Key{"alpha", "1", "A"}, Key{"beta", "1..0", "B"}},
		{"long citation", Key{"alpha", "1", "A"}, Key{"beta", "1", strings.Repeat("B", 65)}},
		{"invalid source", Key{"alpha", "1", " A"}, Key{"beta", "1", "B"}},
		{"unregistered source", Key{"unregistered", "1", "A"}, Key{"beta", "1", "B"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, evidence := range []string{"invented private evidence", ""} {
				in := Input{From: tc.from, To: tc.to, Relation: "related", Source: "invented", Evidence: evidence}
				edges, stats, err := Build(reg, nil, []Input{in, in})
				require.NoError(t, err)
				require.Empty(t, edges)
				require.Equal(t, 2, stats.Dropped)
				require.Equal(t, map[VersionKey]int{{Code: tc.from.Code, Version: tc.from.Version}: 2}, stats.DroppedByVersion)
				require.Zero(t, stats.Unresolved)
				require.Zero(t, stats.Duplicates)
			}
		})
	}
}
