//go:build phapdien

package taxonomy

import (
	"archive/zip"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestReadPackage_Real(t *testing.T) {
	path := os.Getenv("PHAPDIEN_PACKAGE")
	if path == "" {
		t.Fatal("PHAPDIEN_PACKAGE is required")
	}
	p, err := ReadPackage(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.ChuDe) != 45 || len(p.DeMuc) != 293 || len(p.Tree) != 90316 || len(p.Articles) != 86389 {
		t.Fatalf("counts: subjects %d sections %d nodes %d articles %d", len(p.ChuDe), len(p.DeMuc), len(p.Tree), len(p.Articles))
	}
	removed := 0
	for _, d := range p.DeMuc {
		if d.Removed {
			removed++
		}
	}
	if removed != 11 {
		t.Errorf("removed = %d", removed)
	}
	kinds := map[NodeKind]int{}
	for _, n := range p.Tree {
		kinds[n.Kind]++
	}
	for k, want := range map[NodeKind]int{KindDieu: 86389, KindChuong: 1887, KindMuc: 1644, KindTieuMuc: 362, KindPhan: 34} {
		if kinds[k] != want {
			t.Errorf("kind %s = %d, want %d", k, kinds[k], want)
		}
	}
	linked := 0
	for _, a := range p.Articles {
		if a.ItemID != "" {
			linked++
		}
	}
	// Verified against the package on 2026-11-12.
	if linked != 84982 {
		t.Errorf("articles with ItemID = %d", linked)
	}
}

func TestReadPackage_RealArticleNotes(t *testing.T) {
	path := os.Getenv("PHAPDIEN_PACKAGE")
	if path == "" {
		t.Fatal("PHAPDIEN_PACKAGE is required")
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = z.Close() }()
	articleWithNote := regexp.MustCompile(`(?s)<p class='pDieu'><a name='[^']+'></a>.*?</p>(?:\s*<p class='pGhiChu'>(.*?)</p>)?`)
	count := 0
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, "demuc/") || !strings.HasSuffix(f.Name, ".html") || strings.Contains(strings.TrimPrefix(f.Name, "demuc/"), "/") {
			continue
		}
		data, err := readEntry(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range articleWithNote.FindAllSubmatchIndex(data, -1) {
			if match[2] >= 0 {
				count++
			}
		}
	}
	if count != 86388 {
		t.Errorf("articles with notes = %d, want 86388", count)
	}
}

func TestArticleResolver_Real(t *testing.T) {
	path := os.Getenv("PHAPDIEN_PACKAGE")
	if path == "" {
		t.Fatal("PHAPDIEN_PACKAGE is required")
	}
	pkg, err := ReadPackage(path)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := Import(pkg, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewArticleResolver(pkg, mapping)
	retired := map[string]bool{}
	for _, d := range pkg.DeMuc {
		retired[d.GUID] = d.Removed
	}
	resolved, unresolved := 0, 0
	for _, a := range pkg.Articles {
		_, ok := resolver.Code(a)
		if ok {
			resolved++
		} else {
			unresolved++
		}
		if ok == retired[a.DeMucGUID] {
			t.Errorf("article resolution does not match section status")
		}
	}
	t.Logf("resolved %d, unresolved %d", resolved, unresolved)
}
