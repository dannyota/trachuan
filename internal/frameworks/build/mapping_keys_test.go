package build

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/parse"
	"github.com/stretchr/testify/require"
)

func relateKeyFixture(t *testing.T, mappings []parse.Mapping) string {
	t.Helper()
	fixture(t)
	source, work, err := prepare()
	require.NoError(t, err)
	base := filepath.Join(work, "20990101")
	for _, step := range []string{"snapshot", "parse"} {
		require.NoError(t, os.MkdirAll(filepath.Join(base, step), 0700))
	}
	_, err = snapshot(context.Background(), filepath.Join(base, "snapshot"), source)
	require.NoError(t, err)
	require.NoError(t, parseStep(context.Background(), base, filepath.Join(base, "parse")))
	setRelateMappings(t, base, mappings)
	return base
}

func setRelateMappings(t *testing.T, base string, mappings []parse.Mapping) {
	t.Helper()
	path := filepath.Join(base, "parse/catalogs.jsonl")
	rows := readRows[catalog](t, path)
	for i := range rows {
		for j := range rows[i].Nodes {
			rows[i].Nodes[j].Mappings = nil
			rows[i].Nodes[j].DroppedMappings = 0
		}
	}
	rows[0].Nodes[1].Mappings = mappings
	var data []byte
	for _, row := range rows {
		encoded, err := json.Marshal(row)
		require.NoError(t, err)
		data = append(data, append(encoded, '\n')...)
	}
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func TestRelate_DropsInvalidBusinessKeys(t *testing.T) {
	for _, tc := range []struct{ name, code, version, citation string }{
		{"publisher document", "Invented Publisher Document", "v1", "A-2"},
		{"unregistered code", "fw-unregistered", "v1", "A-2"},
		{"long citation", "fw-beta", "v1", strings.Repeat("X", 65)},
		{"malformed version", "fw-beta", "1..0", "A-2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := relateKeyFixture(t, []parse.Mapping{{TargetFramework: tc.code, TargetVersion: tc.version, TargetCitation: tc.citation, Relation: "related"}})
			dir := t.TempDir()
			require.NoError(t, relateStep(context.Background(), base, dir))
			edges, err := os.ReadFile(filepath.Join(dir, "edges.jsonl"))
			require.NoError(t, err)
			require.Empty(t, bytes.TrimSpace(edges))
			gaps := readRows[Gap](t, filepath.Join(dir, "gaps.jsonl"))
			require.Equal(t, []Gap{{Kind: "mapping_dropped", Code: "fw-alpha", Version: "v1", Count: 1}}, gaps)
		})
	}
}

func TestRelate_OldRecipeStopsResume(t *testing.T) {
	work := fixture(t)
	base := filepath.Join(work, "20990101")
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	path := filepath.Join(base, "relate/manifest.json")
	m := readRows[manifest](t, path)[0]
	m.Recipe.Version = 3
	data, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	require.ErrorContains(t, testRun(context.Background(), "20990101", &bytes.Buffer{}), "relate: inputs or recipe changed")
}
