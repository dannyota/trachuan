package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

func TestRelate_DropsUnregisteredEvidenceEnds(t *testing.T) {
	for _, registeredTarget := range []bool{false, true} {
		name := "unregistered framework"
		if registeredTarget {
			name = "unregistered version"
		}
		t.Run(name, func(t *testing.T) {
			base := relateKeyFixture(t, nil)
			path := filepath.Join(base, "snapshot/sources/frameworks.yaml")
			reg, err := readRegistry(path)
			require.NoError(t, err)
			reg.Frameworks[0].Code, reg.Frameworks[0].Version = "nist80053", "r5"
			if registeredTarget {
				reg.Frameworks[1].Code, reg.Frameworks[1].Version = "iso27001", "2021"
			}
			rows := readRows[catalog](t, filepath.Join(base, "parse/catalogs.jsonl"))
			for i := range rows {
				rows[i].Code, rows[i].Version = reg.Frameworks[i].Code, reg.Frameworks[i].Version
			}
			require.NoError(t, os.Remove(filepath.Join(base, "parse/catalogs.jsonl")))
			require.NoError(t, writeRows(filepath.Join(base, "parse"), "catalogs.jsonl", rows))
			f := excelize.NewFile()
			t.Cleanup(func() { require.NoError(t, f.Close()) })
			require.NoError(t, f.SetSheetRow("Sheet1", "A1", &[]string{"Focal Document Element", "Reference Document Element", "Comments (Optional)"}))
			for _, cell := range []string{"A2", "A3"} {
				require.NoError(t, f.SetSheetRow("Sheet1", cell, &[]string{"AC-1", "5.2", "invented private evidence"}))
			}
			buffer, err := f.WriteToBuffer()
			require.NoError(t, err)
			data := buffer.Bytes()
			require.NoError(t, os.WriteFile(filepath.Join(base, "snapshot/files/mapping.xlsx"), data, 0600))
			reg.Frameworks[0].Files = append(reg.Frameworks[0].Files, registry.File{Path: "mapping.xlsx", SHA256: digest(data), Size: int64(len(data)), Role: "mapping workbook", Format: "xlsx", Parser: "nist-olir"})
			encoded, err := yaml.Marshal(reg)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, encoded, 0600))
			dir := t.TempDir()
			require.NoError(t, relateStep(context.Background(), base, dir))
			edges, err := os.ReadFile(filepath.Join(dir, "edges.jsonl"))
			require.NoError(t, err)
			require.Empty(t, edges)
			require.Equal(t, []Gap{{Kind: "mapping_dropped", Code: "nist80053", Version: "r5", Count: 2}}, readRows[Gap](t, filepath.Join(dir, "gaps.jsonl")))
		})
	}
}
