package build

import (
	"context"
	"go/ast"
	"slices"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector/gapsource"
)

func TestSummary_UnknownGapRefusesBuild(t *testing.T) {
	for _, key := range []string{"vn:vbpl:1", "vn:vbpl:outside", ""} {
		t.Run(key, func(t *testing.T) {
			gaps := []InventoryGap{{Stage: "extract", ItemKey: key, Kind: "future_structure_gap"}}
			if _, _, err := makeUnitSummary(summaryFixture(), gaps, nil); err == nil || !strings.Contains(err.Error(), "future_structure_gap") {
				t.Fatalf("unknown gap accepted: %v", err)
			}
		})
	}
}

func TestSummary_UnknownGapRefusesGapLoad(t *testing.T) {
	l := loader{gaps: []InventoryGap{{Stage: "build", Kind: "future_structure_gap"}}}
	if err := l.loadGaps(context.Background()); err == nil || !strings.Contains(err.Error(), "future_structure_gap") {
		t.Fatalf("unknown gap accepted without documents: %v", err)
	}
}

func TestSummary_AllCorpusGapKindsClassifiedOnce(t *testing.T) {
	files, err := gapsource.Parse("../../..")
	if err != nil {
		t.Fatal(err)
	}
	classified := map[string]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "structuralSummaryGap" {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if branch, ok := node.(*ast.CaseClause); ok {
					for _, expr := range branch.List {
						for _, kind := range gapsource.ConstantStrings(expr) {
							classified[kind]++
						}
					}
				}
				return true
			})
		}
	}
	for kind, count := range classified {
		if count != 1 {
			t.Errorf("gap %q classified %d times", kind, count)
		}
	}
	kinds := gapsource.Names(files)
	if len(kinds) < 50 {
		t.Fatalf("gap source enumeration incomplete: %d names", len(kinds))
	}
	for _, kind := range kinds {
		if classified[kind] != 1 {
			t.Errorf("corpus gap %q is missing from classification", kind)
		}
	}
	structural := []string{"ambiguous_tree_text", "article_number_out_of_range", "article_sequence", "structure_uncertain", "text_number_mismatch", "tree_headings_only", "tree_headings_partial", "tree_text_partial", "unknown_node_kind", "invalid_ancestry", "no_tree", "text_source_ambiguous"}
	for kind := range classified {
		if !slices.Contains(kinds, kind) {
			t.Errorf("classified gap %q has no source definition", kind)
		}
		row, _, summaryErr := makeUnitSummary(summaryFixture(), []InventoryGap{{ItemKey: "vn:vbpl:1", Kind: kind}}, nil)
		if summaryErr != nil {
			t.Errorf("known gap %q rejected: %v", kind, summaryErr)
			continue
		}
		if (row.Payload.Outline[0].Heading == "") != slices.Contains(structural, kind) {
			t.Errorf("gap %q changed structural classification", kind)
		}
	}
	t.Logf("classified %d corpus gap kinds", len(kinds))
}
