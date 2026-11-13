package extract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"sort"

	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestExtract_OCRRecordedBytes(t *testing.T) {
	for _, name := range []string{"ocr_decree_142_2026.txt", "ocr_consolidated.txt"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			src := ocrFixture(t, true, 0)
			entry := src.entries[strings.Repeat("a", 64)]
			rows, gaps, record := parseOCR(selection{source: "ocr", name: "fixture.pdf", hash: "fixture", entry: &entry}, data)
			var all []byte
			for _, v := range []any{rows, gaps, record} {
				b, e := json.Marshal(v)
				if e != nil {
					t.Fatal(e)
				}
				all = append(all, b...)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(all))
			want := map[string]string{"ocr_decree_142_2026.txt": "90e3c52ab447130a1f0c6dcfbd243c060113d414811a6e342caad24eacdfcb32", "ocr_consolidated.txt": "2ad4849608cd067fb5b176a9d4faa0b5548c60295974febc2fdaeb5513461e46"}[name]
			if got != want {
				t.Fatalf("artifact bytes hash=%s", got)
			}
		})
	}
}

func TestExtract_RecordedSourceTreeArtifactBytes(t *testing.T) {
	var records []struct {
		ID                  string            `json:"id"`
		TranscriptionSHA256 string            `json:"transcription_sha256"`
		Hashes              map[string]string `json:"baseline_sha256"`
	}
	dir := filepath.Join("testdata", "structure")
	data, err := os.ReadFile(filepath.Join(dir, "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		t.Run(record.ID, func(t *testing.T) {
			transcription, err := os.ReadFile(filepath.Join(dir, record.ID+"-content.html"))
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(transcription)); got != record.TranscriptionSHA256 {
				t.Fatalf("transcription hash=%s want %s", got, record.TranscriptionSHA256)
			}
			data, err := os.ReadFile(filepath.Join(dir, record.ID+"-tree.json"))
			if err != nil {
				t.Fatal(err)
			}
			tree, err := vbpl.DecodeTree(context.Background(), bytes.NewReader(data), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
			if err != nil {
				t.Fatal(err)
			}
			rows, gaps, err := extractTree(context.Background(), tree)
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, file := range []struct {
				name  string
				value any
			}{{"provisions.json", rows}, {"report.json", gaps}} {
				encoded, err := json.Marshal(file.value)
				if err != nil {
					t.Fatal(err)
				}
				sum := fmt.Sprintf("%x", sha256.Sum256(encoded))
				if sum != record.Hashes[file.name] {
					t.Errorf("%s bytes hash=%s want %s", file.name, sum, record.Hashes[file.name])
				}
				lines = append(lines, sum+"  "+file.name+"\n")
			}
			sort.Strings(lines)
			manifest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(lines, ""))))
			if manifest != record.Hashes["MANIFEST.sha256"] {
				t.Errorf("manifest bytes hash=%s want %s", manifest, record.Hashes["MANIFEST.sha256"])
			}
		})
	}
}
