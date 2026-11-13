package extract

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestHeadingSelection_TranscriptionAndOCRBytesUnchanged(t *testing.T) {
	// These manifest hashes were measured against dc00aff's extract and
	// transcription code using a read-only Go overlay.
	f := recordedHeadingFixture(t)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(f.HTML)))
	rows, gaps, record := parseTranscription([]byte(f.HTML), "5664_content.html", hash)
	if got := headingArtifactHash(t, rows, gaps, record); got != "f1bfca37bc4ac700b96f11073d7ac7d04a4f37e7669f15ef23d52588215fc38f" {
		t.Fatalf("transcription manifest=%s", got)
	}
	source := ocrFixture(t, true, 0)
	entry := source.entries[sourceHash(source)]
	rows, gaps, record = parseOCR(selection{source: "ocr", name: "recorded.pdf", hash: entry.Text.PdfSha256, entry: &entry}, source.data)
	if got := headingArtifactHash(t, rows, gaps, record); got != "ef651ccd09fca36169b92a79f7961f0179a49af5ff3541cb55092a591cb41549" {
		t.Fatalf("OCR manifest=%s", got)
	}
}

func headingArtifactHash(t *testing.T, rows []provision, gaps []stage.Gap, record *textRecord) string {
	t.Helper()
	art, err := (artifact.Store{Root: t.TempDir()}).Publish(context.Background(), "extract", strings.Repeat("a", 64), strings.Repeat("b", 64), func(dir string) error {
		for name, value := range map[string]any{"provisions.json": rows, "report.json": gaps, "text.json": record} {
			data, e := json.Marshal(value)
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return art.SHA256
}
