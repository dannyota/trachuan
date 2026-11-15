//go:build integration

package build

import (
	"context"
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestInheritance_GateControlsDatabaseAssignments(t *testing.T) {
	for _, tc := range []struct {
		name       string
		seeded     int
		targetCode string
		status     string
		inherited  int64
	}{
		{"unavailable", 99, "1.1.c1", "unavailable", 0},
		{"fail", 100, "1.1.x1", "fail", 0},
		{"pass", 100, "1.1.c1", "pass", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fixture(t)
			input := taxonomyInputFixture()
			for _, doc := range []struct {
				id           string
				consolidated bool
			}{{"1", false}, {"2", true}} {
				key := "vn:vbpl:" + doc.id
				fetch := f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": doc.id, "docNum": doc.id + "/2024/TT", "isConsolidatedDocument": doc.consolidated}}, "files.json": []any{}})
				var articles []chunk.Node
				for i := 1; i <= 102; i++ {
					number := fmt.Sprint(i)
					articles = append(articles, chunk.Node{NodeID: "n" + number, Kind: "dieu", Label: "Điều " + number, Heading: "Heading", Ordinal: i, Path: "dieu-" + number, Text: "Synthetic article"})
					if doc.id == "1" {
						input.seeds = append(input.seeds, taxonomySeed{doc.id, number, "1.1.c1"})
					} else if i <= tc.seeded {
						input.seeds = append(input.seeds, taxonomySeed{doc.id, number, tc.targetCode})
					}
				}
				f.addArtifact(t, "extract", key, map[string]any{"provisions.json": articles, "report.json": []any{}}, fetch.SHA256)
				if doc.consolidated {
					f.addArtifact(t, "relate", key, map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "consolidates", "target_vbpl_id": "1"}}}, f.relateInput(t, key, fetch.SHA256))
				}
			}
			input.overrides = []taxonomy.Override{{DocumentKey: "vn:vbpl:2", Anchor: "dieu-101", Code: "1.1.c1", Exclude: true}}
			counts := loadTaxonomyFixture(t, f, input)
			var inherited int64
			err := f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM taxonomy_assignment WHERE method='inherited'`).Scan(&inherited)
			if err != nil || inherited != tc.inherited {
				t.Fatalf("inherited rows=%d, want %d: %v", inherited, tc.inherited, err)
			}
			if counts.Taxonomy.Inheritance.Status != tc.status || counts.Taxonomy.Inherited != tc.inherited {
				t.Fatalf("stats: %+v", counts.Taxonomy)
			}
			var method string
			var exclude bool
			err = f.corpus.QueryRow(context.Background(), `SELECT a.method,a.exclude FROM taxonomy_assignment a JOIN provision p ON p.id=a.provision_id JOIN document d ON d.id=p.document_id WHERE d.key='vn:vbpl:2' AND p.anchor='dieu-101'`).Scan(&method, &exclude)
			if err != nil || method != "manual" || !exclude {
				t.Fatalf("override %s %v: %v", method, exclude, err)
			}
			var eligible bool
			err = f.corpus.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM taxonomy_assignment a JOIN provision p ON p.id=a.provision_id JOIN document d ON d.id=p.document_id WHERE d.key='vn:vbpl:2' AND p.anchor='dieu-102' AND a.method='inherited')`).Scan(&eligible)
			if err != nil || eligible != (tc.inherited > 0) {
				t.Fatalf("eligible inherited=%t: %v", eligible, err)
			}
		})
	}
}
