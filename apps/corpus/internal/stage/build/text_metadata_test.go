package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTextMetadata_SourceAndComplete(t *testing.T) {
	dir := t.TempDir()
	source, _, err := readTextMetadata(dir)
	if err != nil || source != "source_tree" {
		t.Fatalf("source=%q err=%v", source, err)
	}
	if textComplete(1, "x", []InventoryGap{{Stage: "extract", ItemKey: "x", Kind: "table_text"}}) {
		t.Fatal("table gap marked complete")
	}
	if textComplete(1, "x", []InventoryGap{{Stage: "extract", ItemKey: "x", Kind: "tree_headings_only"}}) {
		t.Fatal("heading-only tree marked complete")
	}
	if textComplete(0, "x", nil) {
		t.Fatal("empty text marked complete")
	}
	if !textComplete(1, "x", nil) {
		t.Fatal("complete text marked incomplete")
	}
}
func TestTextMetadata_LocatorFromRecord(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "text.json"), []byte(`{"text_source":"html_transcription","provisions":[{"NodeID":"tx:1","StartOrdinal":2,"EndOrdinal":4}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	source, locators, err := readTextMetadata(dir)
	if err != nil || source != "html_transcription" || locators["tx:1"] != "block:2-4" {
		t.Fatalf("source=%s locators=%v err=%v", source, locators, err)
	}
}

func TestLoad_OCRLocators(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"text_source":"ocr","provisions":[{"NodeID":"tx:1","StartOrdinal":2,"EndOrdinal":4,"StartLine":3,"EndLine":8}]}`)
	if err := os.WriteFile(filepath.Join(dir, "text.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	_, locators, err := readTextMetadata(dir)
	if err != nil || locators["tx:1"] != "line:3-8" {
		t.Fatalf("locators=%v err=%v", locators, err)
	}
}
