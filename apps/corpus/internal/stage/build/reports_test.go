package build

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const reportsRecord = `- origin: "eval"
  origin_id: "case-2099-1"
  kind: "wrong_text"
  target: { key: "vn:vbpl:1", anchor: "" }
  claim: { code: "reported" }
`

func reportsInputFile(t *testing.T, content string) ReportsInput {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vn-reports.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	input, err := ReadReports("vn", path)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestReadReports_RequiresFileAndRefusesInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vn-reports.yaml")
	if _, err := ReadReports("vn", path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if err := os.WriteFile(path, []byte("- origin: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReports("vn", path); err == nil {
		t.Fatal("invalid file accepted")
	}
}

func TestBuildInput_ReportsOnlyWithRecords(t *testing.T) {
	inv := Inventory{Country: "vn"}
	today := New(Config{}).inputHash("vn", "20990101", inv, correctionInput{})
	for _, content := range []string{"", "[]\n"} {
		cfg := Config{Reports: reportsInputFile(t, content)}
		if got := New(cfg).inputHash("vn", "20990101", inv, correctionInput{}); got != today {
			t.Fatalf("no records (%q) changed the input hash", content)
		}
	}
	one := New(Config{Reports: reportsInputFile(t, reportsRecord)}).inputHash("vn", "20990101", inv, correctionInput{})
	if one == today || one != withReports(today, hashBytes([]byte(reportsRecord))) {
		t.Fatalf("hash = %s", one)
	}
	two := New(Config{Reports: reportsInputFile(t, strings.Replace(reportsRecord, "case-2099-1", "case-2099-2", 1))}).inputHash("vn", "20990101", inv, correctionInput{})
	if two == one {
		t.Fatal("raw bytes do not reach the input hash")
	}
	if withReports(today, "") != today {
		t.Fatal("empty reports changed the hash")
	}
}

func TestReportsArtifact_NoRecordsWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := writeReportsArtifact(dir, reportsInputFile(t, "[]\n")); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("entries = %v, %v", entries, err)
	}
	if hash, err := reportsArtifactHash(dir); err != nil || hash != "" {
		t.Fatalf("hash = %q, %v", hash, err)
	}
}

func TestReportsArtifact_RecordsWriteBytes(t *testing.T) {
	dir := t.TempDir()
	input := reportsInputFile(t, reportsRecord)
	if err := writeReportsArtifact(dir, input); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "reports.yaml"))
	if err != nil || string(data) != reportsRecord {
		t.Fatalf("reports.yaml = %q, %v", data, err)
	}
	if hash, err := reportsArtifactHash(dir); err != nil || hash != input.hash {
		t.Fatalf("hash = %q, %v", hash, err)
	}
}
