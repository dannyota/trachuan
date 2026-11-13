package build

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/textparse"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func TestParsedStructure_RecordedArticleEvidenceUnchanged(t *testing.T) {
	for _, name := range []string{"articles.html", "second.html", "quoted.html", "images.html", "tables.html"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			paragraphs, err := textparse.ParagraphsFromHTML(data)
			if err != nil {
				t.Fatal(err)
			}
			before, after := textparse.ParseArticles(paragraphs), textparse.Parse(paragraphs)
			if !reflect.DeepEqual(before.Scopes, after.Scopes) || !reflect.DeepEqual(before.Gaps, after.Gaps) {
				t.Fatal("article checks changed")
			}
			old := map[string]textparse.Provision{}
			for _, p := range before.Provisions {
				old[p.Anchor] = p
			}
			for _, p := range after.Provisions {
				if p.Kind != "dieu" {
					continue
				}
				article := p
				article.ParentAnchor = ""
				if old[p.Anchor] != article {
					t.Fatalf("article identity or evidence changed at %s", p.Anchor)
				}
				// Existing manual evidence and identity entries still bind the same article.
				key, number, anchor := "vn:vbpl:1", "1/2020/TT-NHNN", p.Anchor
				correction := source.Correction{Key: key, Number: number, Hold: true, Reason: "reviewed text", Evidence: source.CorrectionEvidence{Anchor: anchor, Quote: p.Text}, Reviewed: "2026-11-12"}
				if _, err = validateCorrections([]source.Correction{correction}, map[string]correctionDocument{key: {number: number, provisions: map[string]string{anchor: p.Text}}}); err != nil {
					t.Fatal(err)
				}
				cases := []evaluation.Case{{ID: "article", ExpectedCitations: []evaluation.Expectation{{DocNumber: number}}}}
				mapping := evaluation.IdentityMap{Version: 2, LegacySHA256: "legacy", Entries: []evaluation.MapEntry{{CaseID: "article", ExpectedNumber: number, State: "resolved", Reason: "reviewed text", ReviewedAt: "2026-11-12", Evidence: []evaluation.Evidence{{DocumentKey: key, DocumentNumber: number, NodeID: old[anchor].NodeID, FetchSHA256: "fetch", QuotedSpan: p.Text}}, Alternatives: []evaluation.Alternative{{Country: "vn", DocumentKey: key, DocumentNumber: number, Kind: "provision", Anchor: &anchor}}}}}
				inventory := evaluation.IdentityInventory{Documents: map[string]evaluation.IdentityDocument{key: {Number: number, Anchors: map[string]bool{}, Nodes: map[string]bool{}}}}
				doc := inventory.Documents[key]
				for _, node := range after.Provisions {
					doc.Anchors[node.Anchor] = true
					doc.Nodes[node.NodeID] = true
				}
				report, e := mapping.Validate(cases, "legacy", inventory)
				if e != nil || len(report["article"]) != 0 {
					t.Fatalf("identity validation=%+v: %v", report, e)
				}
				oldReading := validity.Parse("Thông tư", old[anchor].Text)
				newReading := validity.Parse("Thông tư", p.Text)
				if !reflect.DeepEqual(oldReading, newReading) {
					t.Fatalf("validity reading changed at %s", anchor)
				}
			}
		})
	}
}
