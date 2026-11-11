package cli

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/ocrvision"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestVision_Options(t *testing.T) {
	for _, args := range [][]string{nil, {"--max-pages", "0"}, {"--max-pages", "-1"}, {"--max-pages", "1", "--limit-docs", "-1"}, {"--max-pages", "1", "--pdf", "invalid"}, {"--max-pages", "1", "--pdf", strings.Repeat("A", 64)}, {"--max-pages", "1", "--unknown"}} {
		if _, _, err := parseVisionFlags(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	o, pdf, err := parseVisionFlags([]string{"--max-pages", "20", "--limit-docs", "2", "--pdf", strings.Repeat("a", 64), "--dry-run"})
	if err != nil || o.MaxPages != 20 || o.LimitDocs != 2 || !o.DryRun || pdf != strings.Repeat("a", 64) {
		t.Fatalf("options=%+v pdf=%q err=%v", o, pdf, err)
	}
}

type visionLookup struct {
	entries map[string]ocrstore.Entry
	engines []string
}

func (s *visionLookup) Lookup(_ context.Context, hashes, engines []string, _ string) (map[string]ocrstore.Entry, error) {
	s.engines = engines
	out := map[string]ocrstore.Entry{}
	for _, h := range hashes {
		if e, ok := s.entries[h]; ok {
			for _, engine := range engines {
				if e.Text.EngineID == engine {
					out[h] = e
				}
			}
		}
	}
	return out, nil
}

func TestVision_Selection(t *testing.T) {
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	files := []fetch.DocumentFile{{Name: "first.pdf", SHA256: a}, {Name: "duplicate.PDF", SHA256: a}, {Name: "ignore.doc", SHA256: b}}
	if got := visionFiles(files, ""); len(got) != 1 || got[0] != a {
		t.Fatalf("files=%v", got)
	}
	files = append(files, fetch.DocumentFile{Name: "second.pdf", SHA256: b})
	if got := visionFiles(files, ""); len(got) != 0 {
		t.Fatalf("ambiguous=%v", got)
	}
	if got := visionFiles(files, a); len(got) != 1 || got[0] != a {
		t.Fatalf("explicit=%v", got)
	}
	source := &visionLookup{entries: map[string]ocrstore.Entry{a: {Text: store.OcrText{EngineID: "vision-ocr/builtin-latest"}}}}
	docs := []ocrvision.Document{{Hash: a}, {Hash: b}}
	got, err := filterVisionEntries(context.Background(), source, docs, false)
	if err != nil || len(got) != 1 || got[0].Hash != b {
		t.Fatalf("default=%v err=%v", got, err)
	}
	got, err = filterVisionEntries(context.Background(), source, docs[:1], true)
	if err != nil || len(got) != 1 || got[0].Present {
		t.Fatalf("explicit=%v err=%v", got, err)
	}
	source.entries[a] = ocrstore.Entry{Text: store.OcrText{EngineID: ocrvision.EngineID}}
	got, err = filterVisionEntries(context.Background(), source, docs[:1], true)
	if err != nil || len(got) != 1 || !got[0].Present {
		t.Fatal("fresh entry not marked present")
	}
}
