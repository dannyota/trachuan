package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
)

func TestLoadRelations_TextIdentityGate(t *testing.T) {
	for _, tc := range []struct {
		name, issuer, date string
		keep               bool
	}{
		{"issuer differs", "BỘ TÀI CHÍNH", "2016-12-30", false},
		{"issuer missing", "", "2016-12-30", false},
		{"date differs", "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", "2016-12-29", false},
		{"both agree", "  Ngân hàng\n Nhà nước Việt Nam  ", "2016-12-30", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			artifacts := artifact.Store{Root: t.TempDir()}
			publish := func(name, input string, files map[string]any) artifact.Result {
				t.Helper()
				art, err := artifacts.Publish(ctx, name, input, strings.Repeat("b", 64), func(dir string) error {
					for file, value := range files {
						data, err := json.Marshal(value)
						if err != nil {
							return err
						}
						if err = os.WriteFile(filepath.Join(dir, file), data, 0600); err != nil {
							return err
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return art
			}
			source := publish("fetch", strings.Repeat("a", 64), map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": "1"}}, "files.json": []any{}})
			target := publish("fetch", strings.Repeat("c", 64), map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": "2", "agencyName": tc.issuer, "issueDate": tc.date}}, "files.json": []any{}})
			date, src := "2016-12-30", "source"
			rows := []relationRow{{Ordinal: 0, Kind: "suspends", Method: "text_rule/1", TargetID: "2", SourceNode: &src, TargetAnchor: "dieu-8/khoan-9", IssuerCode: "NHNN", StatedIssueDate: &date}}
			input := relate.PairInputHash(source.SHA256, "-")
			relations := publish("relate", input, map[string]any{"relations.json": rows})
			tx := &noneTx{}
			l := loader{tx: tx, artifacts: artifacts, items: map[string]map[string]InventoryItem{
				"fetch": {
					"vn:vbpl:1": {ItemKey: "vn:vbpl:1", State: "done", Path: &source.Path, ArtifactSHA256: &source.SHA256},
					"vn:vbpl:2": {ItemKey: "vn:vbpl:2", State: "done", Path: &target.Path, ArtifactSHA256: &target.SHA256},
				},
				"extract": {"vn:vbpl:1": {State: "gap"}},
				"relate":  {"vn:vbpl:1": {ItemKey: "vn:vbpl:1", State: "done", Path: &relations.Path, ArtifactSHA256: &relations.SHA256, InputHash: &input}},
			}, docs: map[string]*loadedDocument{
				"vn:vbpl:1": {id: 1, key: "vn:vbpl:1", nodes: map[string]int64{"source": 1}},
				"vn:vbpl:2": {id: 2, key: "vn:vbpl:2", nodes: map[string]int64{"clause": 3, "article": 2}, anchors: map[string]string{"article": "dieu-8", "clause": "dieu-8/khoan-9"}},
			}}
			if err := l.loadRelations(ctx, map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"}); err != nil {
				t.Fatal(err)
			}
			if tc.keep {
				if l.counts.Relations != 1 || len(l.gaps) != 0 {
					t.Fatalf("relations=%d gaps=%+v", l.counts.Relations, l.gaps)
				}
			} else {
				if l.counts.Relations != 0 || len(l.gaps) != 1 {
					t.Fatalf("relations=%d gaps=%+v", l.counts.Relations, l.gaps)
				}
				gap := l.gaps[0]
				if gap.Stage != "build" || gap.ItemKey != "vn:vbpl:1" || gap.Kind != "relation_text_identity_conflict" || gap.Detail != fmt.Sprintf("%s:0", relations.SHA256) {
					t.Fatalf("gap=%+v", gap)
				}
			}
		})
	}
}
