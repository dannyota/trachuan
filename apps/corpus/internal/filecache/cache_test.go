package filecache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLookup_HitAndMisses(t *testing.T) {
	dir := t.TempDir()
	index := "vbpl_id\tname\tsize\tsha256\tformat\tcollected_at\n42\ta.pdf\t3\tba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\tpdf\t2026-04-15T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "index.tsv"), []byte(index), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, id, file string
		size           int64
		date           time.Time
		hit            bool
	}{
		{"hit", "42", "a.pdf", 3, date, true}, {"id", "43", "a.pdf", 3, date, false}, {"name", "42", "b.pdf", 3, date, false}, {"size", "42", "a.pdf", 4, date, false}, {"newer", "42", "a.pdf", 3, date.Add(48 * time.Hour), false}, {"missing date", "42", "a.pdf", 3, time.Time{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, hash, ok := c.Lookup(tc.id, tc.file, tc.size, tc.date)
			if ok != tc.hit {
				t.Fatalf("hit = %v", ok)
			}
			if ok && (hash != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" || path != filepath.Join(dir, "files", hash+".pdf")) {
				t.Fatalf("path %s hash %s", path, hash)
			}
		})
	}
}
