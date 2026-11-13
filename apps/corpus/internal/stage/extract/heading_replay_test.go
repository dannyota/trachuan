package extract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func addCheckedPDF(t *testing.T, dir string, ocr fakeOCR) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "files.json"))
	if err != nil {
		t.Fatal(err)
	}
	var files []fetch.DocumentFile
	if err = json.Unmarshal(data, &files); err != nil {
		t.Fatal(err)
	}
	files = append(files, fetch.DocumentFile{Name: "scan.pdf", SHA256: sourceHash(ocr), Size: 1})
	data, err = json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "files.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSelection_RecordedPartialNeverFallsToOCR(t *testing.T) {
	f := recordedHeadingFixture(t)
	source := ocrFixture(t, true, 0)
	dir := transcriptionBundle(t, string(f.Tree), "")
	addCheckedPDF(t, dir, source)
	s, nodes, err := chooseWithOCR(context.Background(), dir, "vn:vbpl:1", source, AcceptedEngines(), "checker")
	if err != nil || s.source != "source_tree" || s.gap != "tree_headings_partial" || s.consulted || len(nodes) == 0 {
		t.Fatalf("selection=%+v err=%v", s, err)
	}
}

func TestFrozenSelection_RecordedHeadingDecisionsReplay(t *testing.T) {
	f := recordedHeadingFixture(t)
	for _, switches := range []bool{true, false} {
		html := f.HTML
		if !switches {
			html = strings.ReplaceAll(html, "5/12/2000", "6/12/2000")
		}
		dir := transcriptionBundle(t, string(f.Tree), html)
		q := &frozenDB{rows: map[store.GetRunOCRSelectionParams]store.GetRunOCRSelectionRow{}}
		choose := func() (selection, error) {
			s, _, err := chooseWithOCR(context.Background(), dir, "vn:vbpl:1", ocrFixture(t, true, 0), AcceptedEngines(), "checker")
			return s, err
		}
		first, err := planFrozenSelection(context.Background(), q, 1, "vn:vbpl:1", choose)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := planFrozenSelection(context.Background(), q, 1, "vn:vbpl:1", choose)
		if err != nil || replay.source != first.source || replay.gap != first.gap || selectionInputHash("fetch", replay) != selectionInputHash("fetch", first) {
			t.Fatalf("first=%+v replay=%+v err=%v", first, replay, err)
		}
		if (replay.source == "html_transcription") != switches {
			t.Fatalf("source=%s switch=%v", replay.source, switches)
		}
		if !switches {
			materialized, nodes, err := materializeFrozen(context.Background(), dir, "vn:vbpl:1", selection{source: "source_tree", gap: replay.gap, consulted: true})
			if err != nil || materialized.reason != first.reason || len(nodes) == 0 {
				t.Fatalf("materialized=%+v err=%v", materialized, err)
			}
		}
	}
}

func TestFrozenSelection_RecordedLegacyWholeTreeUsesOCR(t *testing.T) {
	f := recordedHeadingFixture(t)
	nodes, err := vbpl.DecodeTree(context.Background(), strings.NewReader(string(f.Tree)), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"success": true, "data": wireNodes(nodes[:1])})
	if err != nil {
		t.Fatal(err)
	}
	dir := transcriptionBundle(t, string(data), "")
	source := ocrFixture(t, true, 0)
	addCheckedPDF(t, dir, source)
	selected, _, err := chooseWithOCR(context.Background(), dir, "vn:vbpl:1", source, AcceptedEngines(), "checker")
	if err != nil || selected.source != "ocr" || !selected.consulted {
		t.Fatalf("selection=%+v err=%v", selected, err)
	}
	// Frozen OCR materialization reads the pinned entry, without selecting again.
	replay, _, err := materializeFrozen(context.Background(), dir, "vn:vbpl:1", selected)
	if err != nil || replay.source != "ocr" || replay.entry != selected.entry || replay.hash != selected.hash {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}
