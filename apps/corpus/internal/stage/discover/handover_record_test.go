package discover

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReadHandOverRecord_Identity(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"candidate":{"id":"123","state":"seed_primary","probe_state":"complete","number":"N","issuer_form":"I","date":"2026-01-01","primary_id":"7","comparisons":[{"id":"7","state":"match","item":{"source_id":"7","raw_number":"N","issuer":"I","issue_date":"2026-01-01"}}]}}`)
	if err := os.WriteFile(filepath.Join(dir, "candidate-123.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadHandOverRecord(dir, "123")
	if err != nil || got.State != "seed_primary" || got.ProbeState != "complete" || got.Number != "N" || got.Issuer != "I" || got.Date != "2026-01-01" || got.PrimaryID != "7" || got.Path != "candidate-123.json" || got.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || len(got.Comparisons) != 1 {
		t.Fatalf("record=%+v err=%v", got, err)
	}
	c := got.Comparisons[0]
	if c.ID != "7" || c.State != "match" || c.Number == nil || *c.Number != "N" || c.Issuer == nil || *c.Issuer != "I" || c.Date == nil || *c.Date != "2026-01-01" {
		t.Fatalf("comparison=%+v", c)
	}
}

func TestReadHandOverRecord_RejectsInvalidID(t *testing.T) {
	for _, id := range []string{"", "../123", "0", "0123"} {
		if _, err := ReadHandOverRecord(t.TempDir(), id); err == nil {
			t.Errorf("accepted id %q", id)
		}
	}
}

func TestReadHandOverRecord_References(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"candidate":{"id":"123","detail_path":"detail/p123-1","detail_hash":"abc","probe_passes":[{"version":1,"items":[],"evidence":{"completeness":"complete","pages_fetched":[{"path":"probe-123/p1-1","sha256":"def"}]}}]}}`)
	if err := os.WriteFile(filepath.Join(dir, "candidate-123.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadHandOverRecord(dir, "123")
	if err != nil || len(got.Files) != 2 || got.Files[0].Path != "detail/p123-1" || got.Files[1].SHA256 != "def" {
		t.Fatalf("record=%+v err=%v", got, err)
	}
}
