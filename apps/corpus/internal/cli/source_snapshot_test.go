package cli

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestSourceSnapshot_PreservesCorrectionBytes(t *testing.T) {
	corrections := []byte("[]\n# reviewed\n")
	snapshot, err := sourceSnapshot([]byte("country: vn\n"), corrections)
	if err != nil {
		t.Fatal(err)
	}
	list, err := source.Parse(snapshot)
	if err != nil || list.CorrectionsBytes != string(corrections) {
		t.Fatalf("corrections = %q, error = %v", list.CorrectionsBytes, err)
	}
}

func TestSourceSnapshot_RejectsUnimplementedCorrections(t *testing.T) {
	_, err := sourceSnapshot([]byte("country: vn\n"), []byte("- key: vn:vbpl:1\n  number: example\n  hold: true\n  reason: review\n  evidence: {quote: example}\n  reviewed: '2026-11-10'\n"))
	if err == nil {
		t.Fatal("accepted corrections without build application")
	}
}
