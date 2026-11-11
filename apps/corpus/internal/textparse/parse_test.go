package textparse

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fixture struct {
	Scopes []struct {
		Kind         string   `json:"kind"`
		Accepted     bool     `json:"accepted"`
		Articles     []string `json:"articles"`
		AnchorPrefix string   `json:"anchor_prefix"`
	} `json:"scopes"`
	Gaps           []string `json:"gaps"`
	GapsMayInclude []string `json:"gaps_may_include"`
	NeutralOnly    bool     `json:"neutral_only"`
	Boundaries     []string `json:"boundaries"`
}

func TestParse_Fixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases map[string]fixture
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			var p []Paragraph
			if strings.HasSuffix(name, ".html") {
				p, err = ParagraphsFromHTML(data)
			} else {
				p, err = ParagraphsFromText(string(data))
			}
			if err != nil {
				t.Fatal(err)
			}
			got := Parse(p)
			var boundaries []string
			seen := map[string]bool{}
			for _, v := range got.Provisions {
				if seen[v.Anchor] {
					t.Errorf("duplicate anchor %q", v.Anchor)
				}
				seen[v.Anchor] = true
				h := sha256.Sum256([]byte(v.Text))
				boundaries = append(boundaries, hex.EncodeToString(h[:]))
			}
			if !reflect.DeepEqual(nonNil(boundaries), nonNil(want.Boundaries)) {
				encoded, _ := json.Marshal(boundaries)
				t.Errorf("boundaries: %s", encoded)
			}
			if name == "empty.html" {
				if !reflect.DeepEqual(got.Gaps, []string{"no_text"}) {
					t.Fatalf("gaps: %v", got.Gaps)
				}
				return
			}
			if len(got.Scopes) != len(want.Scopes) {
				t.Errorf("scopes: %+v, want %+v", got.Scopes, want.Scopes)
			}
			for i, s := range want.Scopes {
				if i >= len(got.Scopes) {
					break
				}
				if got.Scopes[i].Kind != s.Kind || got.Scopes[i].Accepted != s.Accepted {
					t.Errorf("scope %d: %+v want %+v", i, got.Scopes[i], s)
				}
				var articles []string
				for _, v := range got.Provisions {
					if v.ScopeIndex == i && v.Kind == "dieu" {
						articles = append(articles, v.Number)
						if s.AnchorPrefix != "" && !strings.HasPrefix(v.Anchor, s.AnchorPrefix+"/") {
							t.Errorf("anchor %q", v.Anchor)
						}
					}
				}
				if !reflect.DeepEqual(articles, s.Articles) {
					t.Errorf("scope %d articles: %v want %v", i, articles, s.Articles)
				}
			}
			if want.NeutralOnly {
				for _, v := range got.Provisions {
					if v.Kind == "dieu" {
						t.Errorf("unexpected article %+v", v)
					}
				}
			}
			if want.GapsMayInclude == nil && !reflect.DeepEqual(nonNil(got.Gaps), nonNil(want.Gaps)) {
				t.Errorf("gaps: %v want %v", got.Gaps, want.Gaps)
			}
			if want.GapsMayInclude != nil {
				for _, gap := range got.Gaps {
					if !contains(want.GapsMayInclude, gap) {
						t.Errorf("unexpected gap %q", gap)
					}
				}
			}
		})
	}
}
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
func paragraphs(s string) []Paragraph {
	p, err := ParagraphsFromText(s)
	if err != nil {
		panic(err)
	}
	return p
}
func long(s string) string { return s + "\n" + strings.Repeat("nội dung ", 45) }
func TestParse_SequenceGapFallsBackToNeutral(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nĐiều 3. Ba")))
	if len(r.Scopes) != 1 || r.Scopes[0].Accepted || !reflect.DeepEqual(r.Gaps, []string{"structure_uncertain"}) {
		t.Fatalf("%+v", r)
	}
	for _, p := range r.Provisions {
		if p.Kind == "dieu" {
			t.Fatal(p)
		}
	}
}
func TestParse_InsertedArticleAllowed(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nĐiều 1a. Một a\nĐiều 2. Hai")))
	if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
		t.Fatal(r.Scopes)
	}
}
func TestParse_SecondInstrumentScopedAnchors(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nQUY CHẾ\nĐiều 1. Hai\nQUY ĐỊNH\nĐiều 1. Ba")))
	if len(r.Scopes) != 3 || r.Scopes[2].Kind != "instrument" {
		t.Fatal(r.Scopes)
	}
	found := false
	for _, p := range r.Provisions {
		if p.ScopeIndex == 2 && p.Kind == "dieu" {
			found = true
			if p.Anchor != "kem-theo-2/dieu-1" {
				t.Fatal(p)
			}
		}
	}
	if !found {
		t.Fatal("missing second instrument article")
	}
}
func TestParse_RestartWithoutTitleIsTail(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nĐiều 2. Hai\nĐiều 1. Mẫu")))
	if len(r.Scopes) != 2 || r.Scopes[1].Kind != "tail" {
		t.Fatal(r.Scopes)
	}
}
func TestParse_WrappedReferenceIsNotHeading(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nĐiều 6 của Luật này\nĐiều 2. Hai")))
	if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
		t.Fatal(r.Scopes)
	}
}
func TestParse_NodeIDBindsScopeAnchorText(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một")))
	for _, p := range r.Provisions {
		h := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", p.ScopeIndex, p.Anchor, p.Text)))
		if p.NodeID != "tx:"+hex.EncodeToString(h[:])[:16] {
			t.Fatal(p)
		}
	}
}
func TestParse_ArticleTextContainsWholeArticleOnce(t *testing.T) {
	ps := paragraphs(long("Điều 1. Một\nNội dung thứ nhất\nChương II\nQUY ĐỊNH CHUNG\nĐiều 2. Hai\nNội dung thứ hai"))
	r := Parse(ps)
	var texts []string
	for _, p := range r.Provisions {
		if p.ScopeIndex == 0 {
			texts = append(texts, p.Text)
		}
	}
	var lines []string
	for _, p := range ps {
		lines = append(lines, p.Text)
	}
	if strings.Join(texts, "\n") != strings.Join(lines, "\n") {
		t.Fatalf("%q", texts)
	}
}
func TestParse_TableAndImageMarkers(t *testing.T) {
	p, err := ParagraphsFromHTML([]byte("<table><tr><td>" + strings.Repeat("bảng ", 90) + "</td></tr></table><p><img src='x'></p>"))
	if err != nil {
		t.Fatal(err)
	}
	r := Parse(p)
	if !reflect.DeepEqual(r.Gaps, []string{"table_text", "image_content"}) || r.Counts.Images != 1 {
		t.Fatalf("%+v", r)
	}
}
func TestParse_ShortTextIsNoText(t *testing.T) {
	r := Parse(paragraphs("Điều 1. Ngắn"))
	if !reflect.DeepEqual(r.Gaps, []string{"no_text"}) {
		t.Fatal(r.Gaps)
	}
}
func TestParagraphs_LimitsReturnTypedError(t *testing.T) {
	for _, f := range []func() error{func() error { _, err := ParagraphsFromText(strings.Repeat("x\n", 50001)); return err }, func() error {
		_, err := ParagraphsFromHTML([]byte(strings.Repeat("<div>", 65) + "x" + strings.Repeat("</div>", 65)))
		return err
	}, func() error { _, err := ParagraphsFromHTML([]byte(strings.Repeat("<p>x</p>", 50001))); return err }} {
		var limit *LimitError
		if err := f(); !errors.As(err, &limit) {
			t.Fatalf("expected limit error, got %v", err)
		}
	}
}

func TestParse_StructureContainsOnlyTitle(t *testing.T) {
	p := paragraphs(long("Chương I\nQUY ĐỊNH CHUNG\nĐiều 1. Một\nNội dung\nChương II\nĐiều 2. Hai"))
	r := Parse(p)
	for _, v := range r.Provisions {
		if v.Kind == "chuong" && v.Number == "I" {
			if v.Text != "Chương I\nQUY ĐỊNH CHUNG" {
				t.Fatalf("chapter text %q", v.Text)
			}
			return
		}
	}
	t.Fatal("missing chapter")
}
func TestParagraphs_CellAndLineLocators(t *testing.T) {
	p, err := ParagraphsFromHTML([]byte("<table><tr><td><p>e\u0301</p><p> hai </p></td></tr></table>"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 || p[0].Text != "é hai" || !p[0].Table || p[0].Locator != 1 {
		t.Fatalf("%+v", p)
	}
	p, err = ParagraphsFromText("\n e\u0301  hai \n\nba")
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 2 || p[0].Text != "é hai" || p[0].Locator != 2 || p[1].Locator != 4 {
		t.Fatalf("%+v", p)
	}
}

func TestParse_NodeIDChangesWithInputs(t *testing.T) {
	base := makeProvision(paragraphs("one"), "other", "", "", "", "doan-1", 0)
	cases := []Provision{makeProvision(paragraphs("two"), "other", "", "", "", "doan-1", 0), makeProvision(paragraphs("one"), "other", "", "", "", "doan-2", 0), makeProvision(paragraphs("one"), "other", "", "", "", "doan-1", 1)}
	for _, p := range cases {
		if p.NodeID == base.NodeID {
			t.Fatalf("node id did not change: %+v", p)
		}
	}
}

func contains(ss []string, s string) bool {
	for _, item := range ss {
		if item == s {
			return true
		}
	}
	return false
}

func TestParse_InstrumentTitleClosesWithoutArticleOne(t *testing.T) {
	r := Parse(paragraphs(long("Điều 1. Một\nQUY CHẾ\nĐiều 2. Hai")))
	if len(r.Scopes) != 2 || r.Scopes[0].Kind != "main" || r.Scopes[1].Kind != "tail" {
		t.Fatalf("%+v", r.Scopes)
	}
}

func TestParse_TitleWithExtraWordsAndChapterTitle(t *testing.T) {
	p := paragraphs(long("Điều 1. Một\nQUY CHẾ GIAO DỊCH MỘT CỬA\nChương I\nQUY ĐỊNH CHUNG\nĐiều 1. Hai"))
	r := Parse(p)
	if len(r.Scopes) != 2 || r.Scopes[1].Kind != "instrument" || !r.Scopes[1].Accepted {
		t.Fatalf("%+v", r.Scopes)
	}
}
func TestParse_SignatureTableExcludedFromGap(t *testing.T) {
	p := paragraphs(long("Điều 1. Một"))
	p = append(p, Paragraph{Text: "KT. THỐNG ĐỐC", Ordinal: len(p) + 1, Table: true}, Paragraph{Text: strings.Repeat("bảng ", 500), Ordinal: len(p) + 2, Table: true})
	r := Parse(p)
	if contains(r.Gaps, "table_text") {
		t.Fatalf("%+v", r.Counts)
	}
}

func TestParse_QuotedHeadingsAreText(t *testing.T) {
	for _, pair := range [][2]string{{"\"", "\""}, {"“", "”"}, {"‘", "’"}, {"«", "»"}} {
		mark := pair[0]
		t.Run(mark, func(t *testing.T) {
			r := Parse(paragraphs(long("Điều 1. Một\n" + mark + "Điều 9. Trích dẫn" + pair[1] + "\nĐiều 2. Hai")))
			if len(r.Scopes) != 1 || !r.Scopes[0].Accepted {
				t.Fatalf("%+v", r.Scopes)
			}
		})
	}
}
func TestParse_ProvisionTableCountsExcludeLetterhead(t *testing.T) {
	p := []Paragraph{{Text: strings.Repeat("letterhead ", 100), Ordinal: 1, Table: true}, {Text: "Điều 1. Một", Ordinal: 2}, {Text: strings.Repeat("văn bản ", 50), Ordinal: 3}, {Text: strings.Repeat("bảng ", 20), Ordinal: 4, Table: true}}
	r := Parse(p)
	if r.Counts.TableCharacters != 100 || !contains(r.Gaps, "table_text") {
		t.Fatalf("%+v %v", r.Counts, r.Gaps)
	}
}

func TestParse_PreamblePreservesNonTableText(t *testing.T) {
	p := paragraphs(long("Căn cứ Luật này\nĐiều 1. Một"))
	r := Parse(p)
	for _, v := range r.Provisions {
		if v.ScopeIndex == -1 && v.Text == "Căn cứ Luật này" {
			return
		}
	}
	t.Fatal("missing preamble text")
}

func TestParse_NeutralAnchorsAcrossScopes(t *testing.T) {
	r := Parse(paragraphs("Lời mở đầu\nĐiều 1. Một\nĐiều 3. Ba\nPHỤ LỤC\n" + strings.Repeat("phụ lục ", 50)))
	var anchors []string
	for _, p := range r.Provisions {
		if p.Kind == "other" {
			anchors = append(anchors, p.Anchor)
			h := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", p.ScopeIndex, p.Anchor, p.Text)))
			if p.NodeID != "tx:"+hex.EncodeToString(h[:])[:16] {
				t.Fatalf("node id %s", p.NodeID)
			}
		}
	}
	if !reflect.DeepEqual(anchors, []string{"doan-1", "doan-2", "doan-3", "doan-4"}) {
		t.Fatalf("anchors: %v", anchors)
	}
}
