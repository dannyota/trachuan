package chunk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func TestChunk_ListingOnlyDocTypes(t *testing.T) {
	for _, tc := range []struct {
		name, docType string
		listingOnly   bool
	}{
		{"exact", "Văn bản hệ thống hóa", true},
		{"padded", "\n   Văn bản hệ thống hóa  ", true},
		{"unicode edges", "\u00a0Văn bản hệ thống hóa\u2003", true},
		{"tone placement", "Văn bản hệ thống hoá", false},
		{"consolidated", "Văn bản hợp nhất", false},
		{"case", "văn bản hệ thống hóa", false},
		{"empty", "", false},
		{"decomposed", norm.NFD.String("Văn bản hệ thống hóa"), false},
		{"internal spaces", "Văn bản  hệ thống hóa", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := fixtureTrees()["contained"]
			file, coverage := chunkListingFixture(t, tc.docType, nodes)
			if !tc.listingOnly {
				if len(file.Chunks) == 0 {
					t.Fatal("near miss emitted no chunks")
				}
				for _, gap := range coverage.Gaps {
					if gap.Kind == "listing_only" {
						t.Fatal("near miss classified as listing-only")
					}
				}
				return
			}
			assertListingCoverage(t, tc.docType, nodes, file, coverage)
		})
	}
}

func TestChunk_ListingOnlyCoverage(t *testing.T) {
	fixtures := fixtureTrees()
	fixtures["blank"] = []Node{{NodeID: "blank", Kind: "other", Text: " \t\n\u2003"}}
	for name, nodes := range fixtures {
		t.Run(name, func(t *testing.T) {
			file, coverage := chunkListingFixture(t, "Văn bản hệ thống hóa", nodes)
			assertListingCoverage(t, "Văn bản hệ thống hóa", nodes, file, coverage)
		})
	}
}

func TestChunk_ListingOnlyEmptyTreeKeepsNoTree(t *testing.T) {
	file, coverage := chunkListingFixture(t, "Văn bản hệ thống hóa", []Node{})
	if len(file.Chunks) != 0 || !reflect.DeepEqual(coverage.Gaps, []Gap{{Kind: "no_tree"}}) {
		t.Fatalf("file=%+v coverage=%+v", file, coverage)
	}
}

func assertListingCoverage(t *testing.T, docType string, nodes []Node, file File, coverage Coverage) {
	t.Helper()
	if len(file.Chunks) != 0 || len(coverage.CoveredRanges) != 0 {
		t.Fatalf("listing-only document emitted %d chunks and %d covered ranges", len(file.Chunks), len(coverage.CoveredRanges))
	}
	if !reflect.DeepEqual(coverage.Gaps, []Gap{{Kind: "listing_only", Detail: docType}}) {
		t.Fatalf("gaps=%+v", coverage.Gaps)
	}
	accounted := map[string][]bool{}
	for _, node := range nodes {
		accounted[node.NodeID] = make([]bool, len([]rune(node.Text)))
	}
	mark := func(id string, start, end int) {
		t.Helper()
		flags, ok := accounted[id]
		if !ok || start < 0 || end <= start || end > len(flags) {
			t.Fatalf("invalid source range %s [%d,%d)", id, start, end)
		}
		for i := start; i < end; i++ {
			flags[i] = true
		}
	}
	for _, omission := range coverage.Omissions {
		if omission.Reason != "listing_only" || omission.KeptOrdinal != nil {
			t.Fatalf("omission=%+v", omission)
		}
		mark(omission.NodeID, omission.Start, omission.End)
	}
	for _, span := range coverage.WhitespaceRanges {
		mark(span.NodeID, span.Start, span.End)
	}
	for _, node := range nodes {
		for i, r := range []rune(node.Text) {
			if !accounted[node.NodeID][i] {
				t.Fatalf("unaccounted %s offset %d (space=%v)", node.NodeID, i, unicode.IsSpace(r))
			}
		}
	}
	if err := (&splitter{file: file, coverage: coverage}).checkCoverage(nodes); err != nil {
		t.Fatal(err)
	}
}

func chunkListingFixture(t *testing.T, docType string, nodes []Node) (File, Coverage) {
	t.Helper()
	dir, extract := t.TempDir(), t.TempDir()
	detail := map[string]any{"data": map[string]any{"id": "1", "docType": map[string]any{"name": docType}}}
	for path, value := range map[string]any{
		filepath.Join(dir, "detail.json"):         detail,
		filepath.Join(dir, "files.json"):          []any{},
		filepath.Join(extract, "provisions.json"): nodes,
	} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	file, coverage, err := chunkInputs(dir, extract, "vn:vbpl:1")
	if err != nil {
		t.Fatal(err)
	}
	return file, coverage
}
