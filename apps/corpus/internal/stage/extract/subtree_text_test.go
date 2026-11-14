package extract

import (
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestSubtreeText_AgreesWithChunkRendering(t *testing.T) {
	first := "1. Trách nhiệm bảo vệ tài nguyên và nguồn nước."
	second := "2. Trách nhiệm thanh tra nguồn nước thường xuyên."
	for _, base := range []string{
		"Điều 2. Quản lý nguồn nước",
		"Điều 2. Quản lý nguồn nước\n" + first + "\n" + second,
		"Điều 2. Quản lý nguồn nước\n" + first,
		"Điều 2. Quản lý nguồn nước\n" + second + "\n" + first,
		"Điều 2. Quản lý nguồn nước\n" + first + "\n" + first + "\n" + second,
	} {
		t.Run(base, func(t *testing.T) {
			parent := "article"
			rows := []provision{
				{NodeID: parent, Kind: "dieu", Label: "Điều 2", Heading: "Quản lý nguồn nước", Path: "dieu-2", Text: base},
				{NodeID: "first", ParentNodeID: &parent, Kind: "khoan", Label: "Khoản 1", Ordinal: 1, Path: "dieu-2/khoan-1", Text: first},
				{NodeID: "second", ParentNodeID: &parent, Kind: "khoan", Label: "Khoản 2", Ordinal: 2, Path: "dieu-2/khoan-2", Text: second},
			}
			var nodes []chunk.Node
			for _, row := range rows {
				nodes = append(nodes, chunk.Node{NodeID: row.NodeID, ParentNodeID: row.ParentNodeID, Kind: row.Kind, Label: row.Label, Heading: row.Heading, Ordinal: row.Ordinal, Path: row.Path, Text: row.Text})
			}
			recipe := chunk.Recipe2()
			recipe.MinimumRunes = 0
			file, _, err := chunk.Split(nodes, chunk.Meta{}, recipe)
			if err != nil {
				t.Fatal(err)
			}
			var content []string
			for _, c := range file.Chunks {
				content = append(content, c.Content)
			}
			if got, want := collapse(renderTreeText(rows)[parent]), collapse(strings.Join(content, "\n")); got != want {
				t.Fatalf("subtree=%q chunks=%q", got, want)
			}
		})
	}
}
