package build

import (
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestCorrections_VerifyPinnedBytes(t *testing.T) {
	c, err := newCorrectionsInput([]byte("[]\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	c.bytes[0] = 'x'
	if _, _, err := c.pinned(); err == nil {
		t.Fatal("accepted changed correction bytes")
	}
}

func TestCorrections_PlanningReadsCurrentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrections.yaml")
	if err := os.WriteFile(path, []byte("[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := ReadCorrections(path)
	if err != nil {
		t.Fatal(err)
	}
	next := []byte("[]\n# changed before planning\n")
	if err = os.WriteFile(path, next, 0600); err != nil {
		t.Fatal(err)
	}
	_, got, err := c.pinned()
	if err != nil || got.hash != hashBytes([]byte("[]")) {
		t.Fatalf("hash %s: %v", got.hash, err)
	}
}

func TestCorrections_CanonicalSemanticHash(t *testing.T) {
	one := []byte("- key: b\n  number: B\n  hold: true\n  reason: why\n  evidence: {quote: text}\n  reviewed: '2026-11-10'\n- key: a\n  number: A\n  hold: true\n  reason: why\n  evidence: {quote: text}\n  reviewed: '2026-11-10'\n")
	two := []byte("# comment\n- {reviewed: '2026-11-10', evidence: {quote: text}, reason: why, hold: true, number: A, key: a}\n- {reviewed: '2026-11-10', evidence: {quote: text}, reason: why, hold: true, number: B, key: b}\n")
	a, err := newCorrectionsInput(one, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newCorrectionsInput(two, "")
	if err != nil {
		t.Fatal(err)
	}
	_, ah, err := a.pinned()
	if err != nil {
		t.Fatal(err)
	}
	_, bh, err := b.pinned()
	if err != nil {
		t.Fatal(err)
	}
	if ah.hash != bh.hash {
		t.Fatalf("equivalent entries: %s != %s", ah.hash, bh.hash)
	}
}

func TestCorrections_CanonicalJSONGolden(t *testing.T) {
	entries, err := source.ParseCorrections([]byte("- key: b\n  number: B\n  hold: true\n  reason: why\n  evidence: {quote: text}\n  reviewed: '2026-11-10'\n- key: a\n  number: A\n  hold: true\n  reason: why\n  evidence: {quote: text}\n  reviewed: '2026-11-10'\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := canonicalCorrections(entries)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[{"key":"a","number":"A","set":{},"hold":true,"reason":"why","evidence":{"quote":"text"},"reviewed":"2026-11-10"},{"key":"b","number":"B","set":{},"hold":true,"reason":"why","evidence":{"quote":"text"},"reviewed":"2026-11-10"}]`
	if string(got) != want {
		t.Fatalf("canonical = %s", got)
	}
}

func TestCorrections_SemanticInputHashes(t *testing.T) {
	variants := [][]byte{[]byte("[]\n"), []byte("[]\n# comment\n"), []byte("- {key: a, number: A, hold: true, reason: why, evidence: {quote: text}, reviewed: '2026-11-10'}\n"), []byte("- {key: a, number: A, hold: true, reason: changed, evidence: {quote: text}, reviewed: '2026-11-10'}\n")}
	for _, version := range []int{0, 2} {
		inv := Inventory{Version: version}
		var hashes []string
		for _, data := range variants {
			c, err := newCorrectionsInput(data, "")
			if err != nil {
				t.Fatal(err)
			}
			_, pin, err := c.pinned()
			if err != nil {
				t.Fatal(err)
			}
			hashes = append(hashes, inventoryInputHash("vn", "20990101", inv, "map", pin))
		}
		if hashes[0] != hashes[1] || hashes[1] == hashes[2] || hashes[2] == hashes[3] {
			t.Fatalf("version %d: %v", version, hashes)
		}
	}
}

func TestCorrections_CanonicalIncludesEvidenceKey(t *testing.T) {
	data := []byte("- {key: a, number: A, hold: true, reason: why, evidence: {key: b, anchor: dieu-37/khoan-1, quote: text}, reviewed: '2026-11-10'}\n")
	entries, err := source.ParseCorrections(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := canonicalCorrections(entries)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[{"key":"a","number":"A","set":{},"hold":true,"reason":"why","evidence":{"key":"b","anchor":"dieu-37/khoan-1","quote":"text"},"reviewed":"2026-11-10"}]`
	if string(got) != want {
		t.Fatalf("canonical = %s", got)
	}
}
