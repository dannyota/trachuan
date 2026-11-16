package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHeadingExclusions_RequiredPinnedInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exclusions.yaml")
	if _, err := ReadHeadingExclusions(path); err == nil {
		t.Fatal("accepted missing exclusions")
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	input, err := ReadHeadingExclusions(path)
	if err != nil {
		t.Fatal(err)
	}
	_, first, err := input.pinned()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, second, err := input.pinned()
	if err != nil || first != second {
		t.Fatalf("empty hashes %s %s: %v", first, second, err)
	}
	if err = os.WriteFile(path, []byte("[{key: 'vn:vbpl:1', anchor: dieu-1, reason: operative, reviewed: '2026-11-16'}]"), 0600); err != nil {
		t.Fatal(err)
	}
	_, third, err := input.pinned()
	if err != nil || first == third {
		t.Fatalf("changed input not hashed: %v", err)
	}
	inv := Inventory{}
	a := New(Config{}).inputHash("vn", "20990101", inv, correctionInput{})
	b := New(Config{HeadingExclusions: input}).inputHash("vn", "20990101", inv, correctionInput{})
	if a == b {
		t.Fatal("exclusions absent from build input hash")
	}
}
