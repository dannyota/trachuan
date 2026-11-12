package build

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
)

func TestRelationReaders_ValidatePair(t *testing.T) {
	ctx := context.Background()
	store := artifact.Store{Root: t.TempDir()}
	publish := func(stageName string, files map[string]string) artifact.Result {
		t.Helper()
		a, err := store.Publish(ctx, stageName, strings.Repeat("a", 64), strings.Repeat("b", 64), func(dir string) error {
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	f := publish("fetch", map[string]string{"detail.json": `{"data":{"id":"1"}}`, "files.json": `[]`})
	r := publish("relate", map[string]string{"relations.json": `[]`})
	key := "vn:vbpl:1"
	fetch := InventoryItem{Stage: "fetch", ItemKey: key, State: "done", Path: &f.Path, ArtifactSHA256: &f.SHA256}
	extract := InventoryItem{Stage: "extract", ItemKey: key, State: "gap"}
	extractInput := stage.RecipeHash("extract-input", "1", f.SHA256, "none", "absent", "no_text_source")
	e := publish("extract", map[string]string{"text.json": `{"text_source":"none","gap":"no_text_source"}`, "provisions.json": `[]`})
	done := InventoryItem{Stage: "extract", ItemKey: key, State: "done", Path: &e.Path, ArtifactSHA256: &e.SHA256, InputHash: &extractInput}
	pair := relate.PairInputHash(f.SHA256, "-")
	donePair := relate.PairInputHash(f.SHA256, e.SHA256)
	relation := InventoryItem{Stage: "relate", ItemKey: key, State: "done", Path: &r.Path, ArtifactSHA256: &r.SHA256, InputHash: &pair}
	for _, tc := range []struct {
		name                  string
		hash                  *string
		extract               InventoryItem
		includeExtract, valid bool
	}{
		{"gap pair", &pair, extract, true, true}, {"done pair", &donePair, done, true, true},
		{"changed extract", &pair, done, true, false}, {"fetch only", &f.SHA256, extract, true, false},
		{"missing extract", &donePair, extract, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			relation.InputHash = tc.hash
			items := []InventoryItem{fetch, relation}
			byStage := map[string]map[string]InventoryItem{"fetch": {key: fetch}, "relate": {key: relation}}
			if tc.includeExtract {
				items = append(items, tc.extract)
				byStage["extract"] = map[string]InventoryItem{key: tc.extract}
			}
			l := loader{artifacts: store, items: byStage}
			if err := l.loadRelations(ctx, nil); (err == nil) != tc.valid {
				t.Errorf("loadRelations error = %v", err)
			}
			inv := Inventory{ExpectedKeys: []string{key}, Items: items}
			if _, err := IdentityInventory(inv, store); (err == nil) != tc.valid {
				t.Errorf("IdentityInventory error = %v", err)
			}
		})
	}
}

func TestResolveNumber_RequiresUniqueMatch(t *testing.T) {
	numbers := map[string][]string{"01/2024/TT": {"vn:vbpl:1", "vn:vbpl:2"}, "02/2024/TT": {"vn:vbpl:3"}}
	if got := resolveNumber(numbers, "01/2024/TT"); got != "" {
		t.Fatalf("ambiguous number resolved to %q", got)
	}
	if got := resolveNumber(numbers, "02/2024/TT"); got != "vn:vbpl:3" {
		t.Fatalf("unique number resolved to %q", got)
	}
}

func TestRelations_NumberIndexUsesLoadedDocuments(t *testing.T) {
	l := loader{docs: map[string]*loadedDocument{"vn:vbpl:1": {number: "01/2024/TT"}, "vn:vbpl:2": {number: "02/2024/TT"}}}
	numbers := l.documentNumbers()
	if got := resolveNumber(numbers, "01/2024/TT"); got != "vn:vbpl:1" {
		t.Fatalf("number resolved to %q", got)
	}
}

func TestRelationAnchor_Walk(t *testing.T) {
	for _, tc := range []struct {
		name, anchor string
		extra        map[string]string
		want         int64
		gap          string
	}{
		{name: "article", anchor: "dieu-8", want: 1},
		{name: "clause", anchor: "dieu-8/khoan-9", want: 2},
		{name: "point", anchor: "dieu-8/khoan-9/diem-a", want: 3},
		{name: "missing clause", anchor: "dieu-8/khoan-10", want: 1, gap: "relation_clause_unresolved"},
		{name: "missing point", anchor: "dieu-8/khoan-9/diem-b", want: 2, gap: "relation_clause_unresolved"},
		{name: "missing article", anchor: "dieu-9", gap: "relation_article_unresolved"},
		{name: "ambiguous article", anchor: "dieu-8/khoan-9", extra: map[string]string{"duplicate": "dieu-8~2"}, gap: "relation_anchor_ambiguous"},
		{name: "ambiguous clause", anchor: "dieu-8/khoan-9/diem-a", extra: map[string]string{"duplicate": "dieu-8/khoan-9~2"}, gap: "relation_anchor_ambiguous"},
		{name: "ambiguous point", anchor: "dieu-8/khoan-9/diem-a", extra: map[string]string{"duplicate": "dieu-8/khoan-9/diem-a~2"}, gap: "relation_anchor_ambiguous"},
		{name: "suffixed only", anchor: "dieu-8/khoan-10", extra: map[string]string{"duplicate": "dieu-8/khoan-10~2"}, gap: "relation_anchor_ambiguous"},
		{name: "unrelated sibling", anchor: "dieu-8/khoan-9", extra: map[string]string{"duplicate": "dieu-8/khoan-10~2"}, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &loadedDocument{nodes: map[string]int64{"article": 1, "clause": 2, "point": 3, "duplicate": 4}, anchors: map[string]string{"article": "dieu-8", "clause": "dieu-8/khoan-9", "point": "dieu-8/khoan-9/diem-a"}}
			for k, v := range tc.extra {
				doc.anchors[k] = v
			}
			got, gap := relationAnchor(doc, tc.anchor)
			if gap != tc.gap || (tc.want == 0 && got != nil) || (tc.want != 0 && got != tc.want) {
				t.Fatalf("id=%v gap=%s, want=%d %s", got, gap, tc.want, tc.gap)
			}
		})
	}
	if got, gap := relationAnchor(nil, "dieu-8/khoan-9"); got != nil || gap != "" {
		t.Fatalf("absent target id=%v gap=%s", got, gap)
	}
}

func TestTextRelationIdentity_RequiresIssuerAndStatedDate(t *testing.T) {
	date := "2016-12-30"
	for _, tc := range []struct {
		name, issuer, issue, code string
		stated                    *string
		want                      bool
	}{
		{"matching", "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", date, "NHNN", &date, true},
		{"normalized issuer", "  ngân hàng\n nhà nước việt nam  ", date, "NHNN", nil, true},
		{"different date", "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", "2016-12-29", "NHNN", &date, false},
		{"missing date", "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", "", "NHNN", &date, false},
		{"invalid date", "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", "invalid", "NHNN", &date, false},
		{"different issuer", "BỘ TÀI CHÍNH", date, "NHNN", nil, false},
		{"missing issuer", "", date, "NHNN", nil, false},
		{"unknown code", "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", date, "UBND", nil, false},
		{"missing code", "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", date, "", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := textRelationIdentity(relationRow{IssuerCode: tc.code, StatedIssueDate: tc.stated}, fetch.Document{Issuer: tc.issuer, IssueDate: tc.issue}, map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"})
			if got != tc.want {
				t.Fatalf("identity=%v, want %v", got, tc.want)
			}
		})
	}
}
