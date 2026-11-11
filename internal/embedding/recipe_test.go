package embedding

import (
	"os/exec"
	"regexp"
	"testing"
)

func TestRecipe_HashMatchesService(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  Recipe
		want string
	}{
		{"document", DocumentRecipe1(), "13a832d5b076a4ecac77ea73c35c5fae8fe735c7aa2763c6b74dfef65d14d5ec"},
		{"query", QueryRecipe1(), "63ac35af615196ceda67fda05fb5abdbfd30128ef7275899f4cf621fcaed333c"},
	} {
		if h := tc.got.Hash(); h != tc.want {
			t.Errorf("%s recipe hash = %s, want %s", tc.name, h, tc.want)
		}
	}
}

func TestRecipe_SharedModelFieldsAgree(t *testing.T) {
	d, q := DocumentRecipe1(), QueryRecipe1()
	if d.Model != q.Model || d.ResolvedRevision != q.ResolvedRevision || d.TokenizerSHA256 != q.TokenizerSHA256 ||
		d.Precision != "fp16" || q.Precision != "int8" || d.Dimensions != q.Dimensions || d.Pooling != q.Pooling || d.Normalization != q.Normalization {
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

func TestQueryToolchain_MatchesRecipe(t *testing.T) {
	for _, target := range []string{"queryembed-run", "queryembed-test-ort"} {
		t.Run(target, func(t *testing.T) {
			cmd := exec.Command("make", "-n", target, "QUERYEMBED_GO_VERSION=0.0.0")
			cmd.Dir = "../.."
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make dry run: %v: %s", err, output)
			}
			match := regexp.MustCompile(`GOTOOLCHAIN=(go[0-9.]+)`).FindStringSubmatch(string(output))
			if len(match) != 2 || "go "+match[1][2:] != QueryRecipe1().Language {
				t.Fatalf("make toolchain does not match recipe %q: %s", QueryRecipe1().Language, output)
			}
		})
	}
}
