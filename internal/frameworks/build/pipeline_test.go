package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

func editRegistry(t *testing.T, edit func(*registry.Registry)) {
	t.Helper()
	data, err := os.ReadFile("sources/frameworks.yaml")
	require.NoError(t, err)
	reg, err := registry.Parse(bytes.NewReader(data))
	require.NoError(t, err)
	edit(reg)
	data, err = yaml.Marshal(reg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("sources/frameworks.yaml", data, 0600))
}

func TestRun_CanonicalControlAndChunkOrder(t *testing.T) {
	work := fixture(t)
	editRegistry(t, func(reg *registry.Registry) {
		reg.Frameworks[0].Version = "v10"
		reg.Frameworks[0].Ordinal = 2
		reg.Frameworks[1].Code = "fw-alpha"
		reg.Frameworks[1].Version = "v2"
	})
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	base := filepath.Join(work, "20990101")
	controls := readRows[Control](t, filepath.Join(base, "parse/controls.jsonl"))
	require.Len(t, controls, 6)
	require.Equal(t, "v2", controls[0].Version)
	require.Equal(t, "v10", controls[3].Version)
	for _, start := range []int{0, 3} {
		require.Equal(t, []string{"A", "A-2", "A-2(1)"}, []string{controls[start].CitationNorm, controls[start+1].CitationNorm, controls[start+2].CitationNorm})
	}
	chunks := readRows[Chunk](t, filepath.Join(base, "chunk/chunks.jsonl"))
	keys := make([]string, len(chunks))
	for i, chunk := range chunks {
		keys[i] = chunk.DocumentKey
	}
	require.True(t, sort.StringsAreSorted(keys))
}

func TestRun_WorkbookDroppedCount(t *testing.T) {
	work := fixture(t)
	f := excelize.NewFile()
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	require.NoError(t, f.SetSheetRow("Sheet1", "A1", &[]string{"Focal Document Element", "Reference Document Element", "Comments (Optional)"}))
	require.NoError(t, f.SetSheetRow("Sheet1", "A2", &[]string{"unrecognized control", "another control", "invented mapping text"}))
	buffer, err := f.WriteToBuffer()
	require.NoError(t, err)
	data := buffer.Bytes()
	require.NoError(t, os.WriteFile(filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "mapping.xlsx"), data, 0600))
	editRegistry(t, func(reg *registry.Registry) {
		reg.Frameworks[0].Files = append(reg.Frameworks[0].Files, registry.File{Path: "mapping.xlsx", SHA256: digest(data), Size: int64(len(data)), Role: "mapping workbook", Format: "xlsx", Parser: "nist-olir"})
	})
	var out bytes.Buffer
	require.NoError(t, testRun(context.Background(), "20990101", &out))
	require.NotContains(t, out.String(), "invented mapping text")
	gaps := readRows[Gap](t, filepath.Join(work, "20990101/relate/gaps.jsonl"))
	require.Len(t, gaps, 3)
	require.Equal(t, "mapping_dropped", gaps[0].Kind)
	require.Equal(t, "fw-alpha", gaps[0].Code)
	require.Equal(t, "v1", gaps[0].Version)
	require.Nil(t, gaps[0].Citation)
	require.Equal(t, 1, gaps[0].Count)
}

func TestRun_LicensedTextDoesNotChangePublicChunks(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	path := filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "fw-alpha.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = bytes.ReplaceAll(data, []byte("private body"), []byte("different licensed body"))
	data = bytes.ReplaceAll(data, []byte("publisher private title"), []byte("changed publisher title"))
	require.NoError(t, os.WriteFile(path, data, 0600))
	editRegistry(t, func(reg *registry.Registry) {
		reg.Frameworks[0].Files[0].SHA256 = digest(data)
		reg.Frameworks[0].Files[0].Size = int64(len(data))
	})
	require.NoError(t, testRun(context.Background(), "20990102", &bytes.Buffer{}))
	public := func(rel string) []Chunk {
		var rows []Chunk
		for _, chunk := range readRows[Chunk](t, filepath.Join(work, rel, "chunk/chunks.jsonl")) {
			if chunk.Index == "public" {
				rows = append(rows, chunk)
			}
		}
		return rows
	}
	require.Equal(t, public("20990101"), public("20990102"))
}

func TestRun_ChunksFollowControlOrdinals(t *testing.T) {
	work := fixture(t)
	path := filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "fw-alpha.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte(`"id":"a-2","title"`), []byte(`"id":"a-2","props":[{"name":"label","value":"Z-2"}],"title"`), 1)
	require.NoError(t, os.WriteFile(path, data, 0600))
	editRegistry(t, func(reg *registry.Registry) {
		reg.Frameworks[0].Files[0].SHA256 = digest(data)
		reg.Frameworks[0].Files[0].Size = int64(len(data))
	})
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	rows := readRows[Chunk](t, filepath.Join(work, "20990101/chunk/chunks.jsonl"))
	var citations []string
	for _, row := range rows {
		if row.DocumentKey == "fwp:fw-alpha:v1" {
			citations = append(citations, row.Citation)
		}
	}
	require.Equal(t, []string{"A", "Z-2", "A-2(1)"}, citations)
}

func TestRun_SnapshotsReleaseVersionsOnly(t *testing.T) {
	work := fixture(t)
	editRegistry(t, func(reg *registry.Registry) { reg.Frameworks[1].Parser = "" })
	require.NoError(t, os.Remove(filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "fw-beta.json")))
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	require.NoFileExists(t, filepath.Join(work, "20990101/snapshot/files/fw-beta.json"))
	rows := readRows[Control](t, filepath.Join(work, "20990101/parse/controls.jsonl"))
	require.Len(t, rows, 3)
}

func TestValidateLineage_InvalidParents(t *testing.T) {
	for _, parents := range [][]int{{0}, {-2}, {1}, {1, 0}} {
		nodes := make([]parse.Node, len(parents))
		for i, parent := range parents {
			nodes[i] = parse.Node{Parent: parent, Ordinal: i}
		}
		require.Error(t, validateLineage(nodes))
	}
}
