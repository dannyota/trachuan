package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeadingEvidence_WritesDeterministicScanAndHash(t *testing.T) {
	dir := t.TempDir()
	data := []byte("{\"items\":[]}\n")
	want := fmt.Sprintf("%x", sha256.Sum256(data))
	for range 2 {
		got, err := WriteHeadingScan(dir, data)
		if err != nil || got != want {
			t.Fatalf("hash=%s: %v", got, err)
		}
	}
	for name, wantBytes := range map[string][]byte{"heading-scan.json": data, "heading-scan.sha256": []byte(want + "  heading-scan.json\n")} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, wantBytes) {
			t.Fatalf("%s=%q: %v", name, got, err)
		}
	}
}

func TestHeadingEvidence_RejectsDifferentScan(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteHeadingScan(dir, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteHeadingScan(dir, []byte("changed\n")); err == nil {
		t.Fatal("overwrote a scan")
	}
}

func TestHeadingEvidence_BuildRecordMatchesSigningFixture(t *testing.T) {
	record := map[string]any{}
	recordHeadingEvidence(record, strings.Repeat("a", 64), strings.Repeat("b", 64))
	got, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../../../../deploy/github/testdata/heading-build.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.TrimSpace(want)) {
		t.Fatalf("build heading fields=%s, fixture=%s", got, want)
	}
}
