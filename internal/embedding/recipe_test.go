package embedding

import "testing"

func TestRecipe_HashMatchesService(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  Recipe
		want string
	}{
		{"document", DocumentRecipe1(), "f3e678d4ecd8f703086b04479c582c5d335836f428451fab264e36e916e3b421"},
		{"query", QueryRecipe1(), "40bb0222d776337367a7b08976ba779f7222be02d155db9d0bb56ba01fcd39b4"},
	} {
		if h := tc.got.Hash(); h != tc.want {
			t.Errorf("%s recipe hash = %s, want %s", tc.name, h, tc.want)
		}
	}
}

func TestRecipe_SharedModelFieldsAgree(t *testing.T) {
	d, q := DocumentRecipe1(), QueryRecipe1()
	if d.Model != q.Model || d.ResolvedRevision != q.ResolvedRevision || d.TokenizerSHA256 != q.TokenizerSHA256 ||
		d.Precision != q.Precision || d.Precision != "fp32" || d.Dimensions != q.Dimensions || d.Pooling != q.Pooling || d.Normalization != q.Normalization {
		t.Fatal("document and query recipes disagree on shared model fields")
	}
	if d.Runtime != "document" || q.Runtime != "query" || d.TokenCap != 2048 || q.TokenCap != 256 || d.Dimensions != 1024 {
		t.Fatalf("unexpected recipes: %+v %+v", d, q)
	}
}

func TestRecipe_RejectsOldField(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("recipe with old field was accepted")
		}
	}()
	mustRecipe([]byte(`{"python_version":"3.14.10"}`))
}
