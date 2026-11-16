package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectionGuard_EvidenceAndSourceReadsRequireReadableSets(t *testing.T) {
	classes := map[string]relationClass{"mapping_evidence": textRelation, "source_file": textRelation}
	for _, sql := range []string{mappingEvidenceSQL, statusSourcesSQL} {
		read := queryRead{sql: sql, versions: []int64{3}, hasVersions: true}
		require.NoError(t, checkProjection([]queryRead{read}, classes, []int64{3}))
		require.Error(t, checkProjection([]queryRead{read}, classes, nil))
		read.versions = []int64{}
		require.NoError(t, checkProjection([]queryRead{read}, classes, nil))
		read.hasVersions = false
		require.Error(t, checkProjection([]queryRead{read}, classes, nil))
	}
}
