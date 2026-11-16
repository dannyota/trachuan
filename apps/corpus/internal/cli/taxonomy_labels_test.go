package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestSeededArticle_Mapping(t *testing.T) {
	got := seededArticle("doc", "dieu-1", "abc", []string{"z", "a", "z"})
	want := taxonomy.LabelArticle{Key: "doc", Anchor: "dieu-1", OwnTextSHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", Codes: []string{"a", "z"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestTaxonomyLabels_FileBytes(t *testing.T) {
	var pool []taxonomy.LabelArticle
	for i := 0; i < 300; i++ {
		pool = append(pool, seededArticle(fmt.Sprintf("doc-%03d", i), "dieu-1", "abc", []string{"1.2"}))
	}
	load := func(context.Context, string, string) ([]taxonomy.LabelArticle, error) { return pool, nil }
	var first []byte
	for i := 0; i < 2; i++ {
		path := filepath.Join(t.TempDir(), "labels.json")
		var stderr bytes.Buffer
		if code := runTaxonomyLabelsWith(context.Background(), []string{"vn", "20990101_1", "--out", path}, &stderr, load); code != 0 {
			t.Fatalf("code %d: %s", code, stderr.String())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			t.Fatal("missing trailing newline")
		}
		if i == 0 {
			first = data
		} else if !bytes.Equal(first, data) {
			t.Fatal("file bytes changed")
		}
	}
}
