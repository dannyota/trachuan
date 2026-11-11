package taxonomy

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureZip(t *testing.T, change func(string, []byte) []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pkg.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	err = filepath.Walk("testdata/pkg", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name, err := filepath.Rel("testdata/pkg", path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if change != nil {
			data = change(name, data)
		}
		w, err := z.Create(filepath.ToSlash(name))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadPackage_Synthetic(t *testing.T) {
	p, err := ReadPackage(fixtureZip(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.ChuDe) != 2 || len(p.DeMuc) != 4 || len(p.Tree) != 8 || len(p.Articles) != 3 {
		t.Fatalf("counts: %d %d %d %d", len(p.ChuDe), len(p.DeMuc), len(p.Tree), len(p.Articles))
	}
	if !p.DeMuc[2].Removed || p.DeMuc[2].Name != "Đề mục giả 3" || p.ChuDe[0].GUID != "10000000-0000-0000-0000-000000000001" {
		t.Fatalf("subjects: %+v %+v", p.ChuDe, p.DeMuc)
	}
	kinds := map[NodeKind]int{}
	for _, n := range p.Tree {
		kinds[n.Kind]++
	}
	for k, want := range map[NodeKind]int{KindPhan: 1, KindChuong: 2, KindMuc: 1, KindTieuMuc: 1, KindDieu: 3} {
		if kinds[k] != want {
			t.Errorf("kind %v = %d", k, kinds[k])
		}
	}
	if p.Tree[5].ParentGUID != "a0000000-0000-0000-0000-000000000005" || p.Tree[7].ParentGUID != "" || p.Tree[6].Order <= p.Tree[4].Order {
		t.Errorf("tree: %+v", p.Tree)
	}
	a := p.Articles
	if a[0].ItemID != "123" || a[0].Number != "12" || a[0].TreeGUID != "a0000000-0000-0000-0000-000000000005" || a[1].ItemID != "" || a[1].Number != "34" || a[2].Number != "" || a[0].Seq == a[1].Seq {
		t.Errorf("articles: %+v", a)
	}
}
func TestReadPackage_HashMismatchIsCallerCheck(t *testing.T) {
	path := fixtureZip(t, nil)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ReadPackage(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.SHA256 != fmt.Sprintf("%x", sha256.Sum256(b)) {
		t.Fatal("wrong hash")
	}
}
func TestReadPackage_RejectsUnknownKind(t *testing.T) {
	path := fixtureZip(t, func(name string, b []byte) []byte {
		if name == "jsonData.js" {
			return []byte(strings.Replace(string(b), "Phần giả", "Lạ giả", 1))
		}
		return b
	})
	_, err := ReadPackage(path)
	if err == nil || !strings.Contains(err.Error(), "a0000000-0000-0000-0000-000000000001") {
		t.Fatalf("got %v", err)
	}
}

func TestReadPackage_ArticleNumberAndFirstLink(t *testing.T) {
	path := fixtureZip(t, func(name string, b []byte) []byte {
		if name == "demuc/20000000-0000-0000-0000-000000000002.html" {
			return []byte("<p class='pDieu'><a name='other'></a>Điều giả</p><p class='pGhiChu'><a href='vbpq-toanvan.aspx?ItemID=7#Dieu_12a'>Giả (Điều 23a)</a><a href='vbpq-toanvan.aspx?ItemID=8#Dieu_99'>Giả</a></p>")
		}
		return b
	})
	p, err := ReadPackage(path)
	if err != nil {
		t.Fatal(err)
	}
	a := p.Articles[len(p.Articles)-1]
	if a.ItemID != "7" || a.Number != "" {
		t.Fatalf("got %+v", a)
	}
}
func TestReadPackage_RejectsOversizedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("jsonData.js")
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 1024*1024)
	for i := 0; i < 65; i++ {
		if _, err = w.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = ReadPackage(path)
	if err == nil || !strings.Contains(err.Error(), "64 MB") {
		t.Fatalf("got %v", err)
	}
}

func TestReadPackage_FirstSourceLink(t *testing.T) {
	path := fixtureZip(t, func(name string, b []byte) []byte {
		if name == "demuc/20000000-0000-0000-0000-000000000002.html" {
			return []byte("<p class='pDieu'><a name='other'></a>Điều giả</p><p class='pGhiChu'><a href='other.html'>Khác</a><a href='vbpq-toanvan.aspx#Dieu_8'>Thiếu mã</a><a href='vbpq-toanvan.aspx?ItemID=42#Dieu_17'>Nguồn giả</a><a href='vbpq-toanvan.aspx?ItemID=99#Dieu_18'>Nguồn giả khác</a> (Điều 19)</p>")
		}
		return b
	})
	p, err := ReadPackage(path)
	if err != nil {
		t.Fatal(err)
	}
	a := p.Articles[len(p.Articles)-1]
	if a.ItemID != "42" || a.Number != "17" {
		t.Fatalf("got ItemID %q, Number %q; want 42, 17", a.ItemID, a.Number)
	}
}
