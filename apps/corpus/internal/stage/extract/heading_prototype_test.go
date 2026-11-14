package extract

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestHeadingAgreement_RecordedNestedSubtrees(t *testing.T) {
	data, err := os.ReadFile("testdata/heading-nested-subtrees.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Key, Anchor, Transcription string
		RenderedCharacters         int `json:"rendered_characters"`
		Rows                       []provision
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Key, func(t *testing.T) {
			rendered := renderTreeText(f.Rows)[f.Rows[0].NodeID]
			if utf8.RuneCountInString(rendered) != f.RenderedCharacters || strings.Contains(collapse(f.Transcription), collapse(rendered)) {
				t.Fatalf("rendered characters=%d; want %d and no contiguous agreement", utf8.RuneCountInString(rendered), f.RenderedCharacters)
			}
			var nodes []chunk.Node
			for _, row := range f.Rows {
				nodes = append(nodes, chunk.Node{NodeID: row.NodeID, ParentNodeID: row.ParentNodeID, Kind: row.Kind, Label: row.Label, Heading: row.Heading, Ordinal: row.Ordinal, Path: row.Path, Text: row.Text})
			}
			recipe := chunk.Recipe2()
			recipe.MinimumRunes = 0
			file, coverage, err := chunk.Split(nodes, chunk.Meta{}, recipe)
			if err != nil {
				t.Fatal(err)
			}
			var parts []string
			for _, part := range file.Chunks {
				parts = append(parts, part.Content)
			}
			if collapse(rendered) != collapse(strings.Join(parts, "\n")) {
				t.Fatal("agreement did not preserve the complete chunk rendering")
			}
			ambiguous := false
			for _, gap := range coverage.Gaps {
				ambiguous = ambiguous || gap.Kind == "ambiguous_tree_text"
			}
			if !ambiguous {
				t.Fatal("recorded nested subtree no longer has ambiguous containment")
			}
			var rows []provision
			var body strings.Builder
			var number int
			if _, err = fmt.Sscanf(f.Anchor, "dieu-%d", &number); err != nil {
				t.Fatal(err)
			}
			for i := 1; i < number; i++ {
				rows = append(rows, provision{NodeID: fmt.Sprint("heading-", i), Kind: "dieu", Label: fmt.Sprint("Điều ", i), Path: fmt.Sprint("dieu-", i), Text: fmt.Sprint("Điều ", i, ".")})
				fmt.Fprintf(&body, "<p>Điều %d.</p>", i)
			}
			rows = append(rows, f.Rows...)
			for _, line := range strings.Split(f.Transcription, "\n") {
				body.WriteString("<p>" + html.EscapeString(line) + "</p>")
			}
			comparison := compareTree(rows, selection{source: "html_transcription", data: []byte(body.String())})
			if comparison.FailedRule != 3 {
				t.Fatalf("comparison=%+v", comparison)
			}
		})
	}
}

func TestHeadingAgreement_RejectedMainDoesNotUseInstrument(t *testing.T) {
	for _, tc := range []struct{ name, main, reason string }{
		{"duplicate article", "<p>Điều 1. Một</p><p>Điều 2. Hai</p><p>Điều 3. Ba</p><p>Điều 3. Lặp</p>", "expected next article after 3, found 3"},
		{"main starts at three", "<p>Điều 3. Ba</p>", "expected next article after 0, found 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.main + "<p>QUY CHẾ</p><p>Điều 1. Phạm vi</p><p>" + strings.Repeat("Quy định chung. ", 30) + "</p>"
			_, _, record := parseTranscription([]byte(body), "recorded-scope.html", "")
			if len(record.Scopes) != 2 || record.Scopes[0].Accepted || record.Scopes[0].FailedCheck != tc.reason || !record.Scopes[1].Accepted {
				t.Fatalf("scopes=%+v", record.Scopes)
			}
			comparison := compareTree([]provision{{NodeID: "heading", Kind: "dieu", Label: "Điều 1", Text: "Điều 1.", Path: "dieu-1"}}, selection{source: "html_transcription", data: []byte(body)})
			if comparison.FailedRule != 1 {
				t.Fatalf("comparison=%+v", comparison)
			}
		})
	}
}
