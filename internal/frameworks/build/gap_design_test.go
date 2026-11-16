package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGapKinds_MatchesDesignTable(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../docs/design/frameworks-build.md"))
	require.NoError(t, err)
	require.Equal(t, designGapKinds(t, string(data)), gapKinds())
}

func designGapKinds(t *testing.T, document string) map[string]gapRule {
	t.Helper()
	_, section, ok := strings.Cut(document, "## Gaps and signing\n")
	require.True(t, ok, "gap policy section missing")
	kinds := map[string]gapRule{}
	started := false
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			if started {
				break
			}
			continue
		}
		started = true
		cells := strings.Split(strings.Trim(line, "|"), "|")
		require.Len(t, cells, 3)
		for i, cell := range cells {
			cells[i] = strings.Join(strings.Fields(strings.ReplaceAll(cell, "`", "")), " ")
		}
		if cells[0] == "Kind" || strings.HasPrefix(cells[0], "---") {
			continue
		}
		var blocking string
		switch cells[2] {
		case "no":
			blocking = "never"
		case "yes":
			blocking = "always"
		case "yes for a licensed version, no otherwise":
			blocking = "licensed"
		default:
			t.Fatalf("unsupported gap blocking rule %q", cells[2])
		}
		for _, kind := range strings.Split(cells[0], ",") {
			kind = strings.TrimSpace(kind)
			require.NotEmpty(t, kind)
			require.NotContains(t, kinds, kind)
			kinds[kind] = gapRule{Class: cells[1], Blocking: blocking}
		}
	}
	require.NotEmpty(t, kinds)
	return kinds
}
