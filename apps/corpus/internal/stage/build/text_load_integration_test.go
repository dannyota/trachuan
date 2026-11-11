//go:build integration

package build

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func TestLoad_TextSourceAndComplete(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	node := chunk.Node{NodeID: "n1", Kind: "dieu", Label: "Điều 1", Ordinal: 1, Path: "dieu-1", Text: "Điều 1. Nội dung."}
	for _, tc := range []struct {
		key, source string
		gap         bool
	}{
		{"vn:vbpl:1", "source_tree", false},
		{"vn:vbpl:2", "html_transcription", false},
		{"vn:vbpl:3", "html_transcription", true},
		{"vn:vbpl:4", "none", false},
	} {
		art := f.fetch(t, tc.key)
		if tc.source == "none" {
			continue
		}
		files := map[string]any{"provisions.json": []chunk.Node{node}}
		input := art.SHA256
		if tc.source == "html_transcription" {
			files["text.json"] = map[string]any{"text_source": tc.source, "provisions": []map[string]any{{"NodeID": node.NodeID, "StartOrdinal": 2, "EndOrdinal": 4}}}
			input = fetch.ExtractInputHash(art.SHA256, tc.source)
		}
		f.addArtifact(t, "extract", tc.key, files, input)
		if tc.gap {
			if _, err := f.workshop.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES($1,'extract',$2,'structure_uncertain','')`, f.run.ID, tc.key); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := f.load(t); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key, source string
		complete    bool
	}{
		{"vn:vbpl:1", "source_tree", true}, {"vn:vbpl:2", "html_transcription", true}, {"vn:vbpl:3", "html_transcription", false}, {"vn:vbpl:4", "none", false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			var source string
			var complete bool
			err := f.corpus.QueryRow(ctx, `SELECT text_source,text_complete FROM document WHERE key=$1`, tc.key).Scan(&source, &complete)
			if err != nil || source != tc.source || complete != tc.complete {
				t.Fatalf("source=%q complete=%v: %v", source, complete, err)
			}
		})
	}
	var treeLocator *string
	if err := f.corpus.QueryRow(ctx, `SELECT p.locator FROM provision p JOIN document d ON d.id=p.document_id WHERE d.key='vn:vbpl:1'`).Scan(&treeLocator); err != nil || treeLocator != nil {
		t.Fatalf("source-tree locator=%v: %v", treeLocator, err)
	}
}

func TestLoad_LocatorFromTextRecord(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "present"
		if missing {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			key := "vn:vbpl:1"
			art := f.fetch(t, key)
			node := chunk.Node{NodeID: "tx:1234567890abcdef", Kind: "dieu", Label: "Điều 1", Ordinal: 1, Path: "dieu-1", Text: "Điều 1. Nội dung."}
			entries := []map[string]any{{"NodeID": node.NodeID, "StartOrdinal": 2, "EndOrdinal": 4}}
			if missing {
				entries = nil
			}
			f.addArtifact(t, "extract", key, map[string]any{"provisions.json": []chunk.Node{node}, "text.json": map[string]any{"text_source": "html_transcription", "provisions": entries}}, fetch.ExtractInputHash(art.SHA256, "html_transcription"))
			_, err := f.load(t)
			if missing {
				if err == nil || !strings.Contains(err.Error(), "missing locator") {
					t.Fatalf("missing locator error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var locator *string
			if err = f.corpus.QueryRow(context.Background(), `SELECT locator FROM provision WHERE node_id=$1`, node.NodeID).Scan(&locator); err != nil || locator == nil || *locator != "block:2-4" {
				t.Fatalf("locator=%v: %v", locator, err)
			}
		})
	}
}
