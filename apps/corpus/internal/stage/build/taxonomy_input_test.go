package build

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestReadTaxonomy_NoMapping(t *testing.T) {
	input, err := ReadTaxonomy(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if input.version != "" {
		t.Fatalf("version = %q", input.version)
	}
}

func taxonomyFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	root := filepath.Join("..", "..", "taxonomy", "testdata", "pkg")
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		w, e := z.Create(filepath.ToSlash(name))
		if e != nil {
			return e
		}
		data, e := os.Open(path)
		if e != nil {
			return e
		}
		_, e = io.Copy(w, data)
		closeErr := data.Close()
		if e != nil {
			return e
		}
		return closeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	pkg, err := taxonomy.ReadPackage(path)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := taxonomy.Import(pkg, taxonomy.Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "mapping.yaml"), mapping.Encode(), 0600); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func TestReadTaxonomy_PackageMismatch(t *testing.T) {
	dir, path := taxonomyFixture(t)
	mapping, err := os.ReadFile(filepath.Join(dir, "mapping.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := taxonomy.ReadMapping(mapping)
	if err != nil {
		t.Fatal(err)
	}
	parsed.PackageSHA256 = strings.Repeat("0", 64)
	if err = os.WriteFile(filepath.Join(dir, "mapping.yaml"), parsed.Encode(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadTaxonomy(dir, path); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("got %v", err)
	}
}

func TestReadTaxonomy_Synthetic(t *testing.T) {
	dir, path := taxonomyFixture(t)
	input, err := ReadTaxonomy(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if input.version == "" || len(input.seeds) != 1 || input.seeds[0].ItemID != "123" || input.seeds[0].Number != "12" {
		t.Fatalf("input: %+v", input.seeds)
	}
	if err = os.WriteFile(filepath.Join(dir, "custom.yaml"), []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = input.pinned(); err == nil {
		t.Fatal("accepted changed input bytes")
	}
}

func TestReadTaxonomy_PinnedBytes(t *testing.T) {
	dir := t.TempDir()
	input, err := ReadTaxonomy(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mapping.yaml"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := input.pinned(); err == nil {
		t.Fatal("accepted changed mapping")
	}
}

func TestReadTaxonomy_PinnedPackage(t *testing.T) {
	dir, path := taxonomyFixture(t)
	input, err := ReadTaxonomy(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = input.pinned(); err == nil {
		t.Fatal("accepted changed package")
	}
}

func TestBuild_InputHashIncludesTaxonomy(t *testing.T) {
	inv := Inventory{Version: 2}
	a := New(Config{Taxonomy: TaxonomyInput{version: "one"}}).inputHash("vn", "20990101", inv, correctionInput{})
	b := New(Config{Taxonomy: TaxonomyInput{version: "two"}}).inputHash("vn", "20990101", inv, correctionInput{})
	if a == b {
		t.Fatal("taxonomy version did not change input hash")
	}
	if got := recordInputHash("vn", "20990101", inv, "", hashJSON(inv.Provenance), "", "", "one"); got != a {
		t.Fatalf("record hash = %q, want %q", got, a)
	}
}

func TestTaxonomy_RetiredPlacement(t *testing.T) {
	m := taxonomy.Mapping{Nodes: []taxonomy.MappedNode{
		{Code: "1", Level: 1, Status: "active", Ordinal: 1},
		{Code: "1.1", ParentCode: "1", Level: 2, Status: "active", Ordinal: 1},
		{Code: "1.1.c1", ParentCode: "1.1", Level: 3, Status: "active", Ordinal: 1},
		{Code: "1.1.c1.m1", ParentCode: "1.1.c1", Level: 4, Status: "active", Ordinal: 1},
		{Code: "1.1.c2", ParentCode: "1.1.c1.m1", Level: 3, Status: "retired"},
		{Code: "1.1.c2.m1", ParentCode: "1.1.c2", Level: 4, Status: "retired"},
	}}
	nodes, err := taxonomyNodes(TaxonomyInput{mapping: m})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 4 {
		t.Fatalf("got %d nodes, want 4", len(nodes))
	}
}
