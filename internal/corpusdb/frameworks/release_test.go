package frameworks

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApply_ReleaseHasSeparateStepProvenance(t *testing.T) {
	db := &recordingDB{}
	require.NoError(t, ApplyTables(context.Background(), db, 1024))
	_, release, ok := strings.Cut(db.statements[0], "CREATE TABLE IF NOT EXISTS release (")
	require.True(t, ok)
	release, _, _ = strings.Cut(release, "\n);")
	require.NotContains(t, release, "step_recipes")
	for _, step := range []string{"snapshot", "parse", "relate", "chunk", "embed"} {
		t.Run(step, func(t *testing.T) {
			recipe := step + "_recipe"
			hash := step + "_input_hash"
			require.Regexp(t, `(?m)^\s*`+recipe+` jsonb NOT NULL\b`, release)
			require.Contains(t, release, "jsonb_typeof("+recipe+") = 'object'")
			require.Regexp(t, `(?m)^\s*`+hash+` text COLLATE "C" NOT NULL\b`, release)
			require.Contains(t, release, hash+" ~ '^[0-9a-f]{64}$'")
		})
	}
	columns := regexp.MustCompile(`(?m)^\s*(\w+)\s+jsonb\b`).FindAllStringSubmatch(release, -1)
	require.Len(t, columns, 8)
}
