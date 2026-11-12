package fetch

import (
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestReadDocument_VBPLSourceURL(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"66518", "https://vbpl.vn/van-ban/chi-tiet/66518"},
		{"123e4567-e89b-12d3-a456-426614174000", "https://vbpl.vn/van-ban/chi-tiet/123e4567-e89b-12d3-a456-426614174000"},
		{"vbpqdinhchinh_297", "https://vbpl.vn/van-ban/chi-tiet/vbpqdinhchinh_297"},
		{"vbpqta_11113", ""}, {"VBPQTA_11113", ""}, {"bad/id", ""}, {"bad?id", ""},
	} {
		t.Run(tc.id, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(`{"data":{"id":"`+tc.id+`"}}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[]`), 0600); err != nil {
				t.Fatal(err)
			}
			doc, err := ReadDocument(dir, "vn:vbpl:"+tc.id)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if doc.SourceURL != nil {
				got = *doc.SourceURL
			}
			if got != tc.want || (tc.want == "" && doc.SourceURL != nil) {
				t.Errorf("SourceURL = %v, want %q", doc.SourceURL, tc.want)
			}
		})
	}
}

func TestFetchRecipe_Pinned(t *testing.T) {
	if got := (&Stage{}).Recipe(stage.Snapshot{}); got != "1f6699b0264c7ff2550542ccf2689868c1d40e435464151edcfa3dd991b9b351" {
		t.Fatalf("fetch recipe = %s", got)
	}
}

func TestVBPLSourceURL_Validation(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"42", "https://vbpl.vn/van-ban/chi-tiet/42"},
		{"123e4567-e89b-12d3-a456-426614174000", "https://vbpl.vn/van-ban/chi-tiet/123e4567-e89b-12d3-a456-426614174000"},
		{"vbpqdinhchinh_297", "https://vbpl.vn/van-ban/chi-tiet/vbpqdinhchinh_297"},
		{"vbpqta_11113", ""}, {"VBPQTA_11113", ""}, {"", ""}, {"bad/id", ""}, {"bad?id", ""},
	} {
		if got := vbplSourceURL(tc.id); got != tc.want {
			t.Errorf("vbplSourceURL(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}
