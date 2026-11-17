package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestArticleOptions_RequiresLabeledPilotAndFamilyArtifact(t *testing.T) {
	valid := []string{"--families", "families.json", "--select", "labels:taxonomy-labels.json", "--recipe", "claude-haiku-5-5", "--out", "output", "--budget", "pilot", "--budget-usd", "15", "--dry-run"}
	o, err := parseArticleOptions("vn", "20990101", valid)
	if err != nil || o.Labels != "taxonomy-labels.json" || o.Families != "families.json" || !o.Options.DryRun {
		t.Fatalf("options %+v error %v", o, err)
	}
	for _, args := range [][]string{nil, valid[:8], {"--families", "f", "--select", "all", "--recipe", "claude-haiku-5-5", "--out", "o", "--budget", "pilot", "--budget-usd", "15"}, append(append([]string{}, valid...), "--unknown")} {
		if _, err = parseArticleOptions("vn", "20990101", args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	var out, errout bytes.Buffer
	if code := runClassify(context.Background(), []string{"articles", "vn", "20990101"}, &out, &errout); code != 2 {
		t.Fatalf("exit %d: %s", code, &errout)
	}
}

func TestClassifyArticles_DryRunRefusesIncompleteFamilies(t *testing.T) {
	t.Setenv("CORPUS_SUBMISSIONS_DIR", "")
	dir := t.TempDir()
	labels := taxonomy.Labels{Country: "vn", Release: "20990101"}
	evidence := classificationEvidence{taxonomyHash: strings.Repeat("a", 64), nodes: []classify.Node{{Code: "1.1", Level: 2, Active: true, Origin: "phapdien"}}}
	for i := 1; i <= 300; i++ {
		key := "vn:vbpl:1"
		if i == 300 {
			key = "vn:vbpl:2"
		}
		anchor := fmt.Sprintf("dieu-%d", i)
		labels.Articles = append(labels.Articles, taxonomy.LabelArticle{Key: key, Anchor: anchor, OwnTextSHA256: taxonomy.OwnTextSHA256("Synthetic"), Codes: []string{"1.1"}})
		evidence.articles = append(evidence.articles, classify.Article{DocumentKey: key, Anchor: anchor, OwnText: "Synthetic", Text: "Synthetic"})
	}
	write := func(name string, value any) []byte {
		t.Helper()
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
		return b
	}
	write("labels.json", labels)
	recipe, err := classify.NewRecipe("claude-haiku-5-5", evidence.taxonomyHash)
	if err != nil {
		t.Fatal(err)
	}
	b := write("families.json", classify.FamilyArtifact{Version: 1, Recipe: recipe, RecipeHash: recipe.Hash(), TaxonomyHash: evidence.taxonomyHash, Documents: map[string]classify.Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}})
	if err = os.WriteFile(filepath.Join(dir, "MANIFEST.sha256"), fmt.Appendf(nil, "%x  families.json\n", sha256.Sum256(b)), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"vn", "20990101", "--families", filepath.Join(dir, "families.json"), "--select", "labels:" + filepath.Join(dir, "labels.json"), "--recipe", recipe.Model, "--out", filepath.Join(dir, "output"), "--budget", "pilot", "--budget-usd", "15", "--dry-run"}
	var out, errout bytes.Buffer
	code := runClassifyArticlesWith(context.Background(), args, &out, &errout, func(context.Context, string, string, taxonomy.Labels) (classificationEvidence, error) {
		return evidence, nil
	})
	if code != 1 || !strings.Contains(errout.String(), "vn:vbpl:2 missing from families artifact") || out.Len() != 0 {
		t.Fatalf("code %d stdout %s stderr %s", code, &out, &errout)
	}
	if _, err = os.Stat(filepath.Join(dir, "output")); !os.IsNotExist(err) {
		t.Fatalf("incomplete families wrote output: %v", err)
	}
}

func TestScoreOptions_RequiresEveryArtifact(t *testing.T) {
	valid := []string{"--labels", "labels.json", "--families", "families.json", "--articles", "articles.json", "--out", "output"}
	o, err := parseScoreOptions("vn", "20990101", valid)
	if err != nil || o.Labels != "labels.json" || o.Out != "output" {
		t.Fatalf("options %+v error %v", o, err)
	}
	if _, err = parseScoreOptions("vn", "20990101", valid[:6]); err == nil {
		t.Fatal("accepted missing output")
	}
	var out, errout bytes.Buffer
	if code := runClassify(context.Background(), []string{"score", "vn", "20990101"}, &out, &errout); code != 2 {
		t.Fatalf("exit %d: %s", code, &errout)
	}
}

func TestArticleOptions_MaxTokens(t *testing.T) {
	base := []string{"--families", "families.json", "--select", "labels:taxonomy-labels.json", "--recipe", "claude-sonnet-5-5", "--out", "output", "--budget", "pilot", "--budget-usd", "15"}
	o, err := parseArticleOptions("vn", "20990101", base)
	if err != nil || o.MaxTokens != 0 {
		t.Fatalf("default options %+v error %v", o, err)
	}
	o, err = parseArticleOptions("vn", "20990101", append(append([]string{}, base...), "--max-tokens", "1452"))
	if err != nil || o.MaxTokens != 1452 {
		t.Fatalf("max tokens options %+v error %v", o, err)
	}
	for _, v := range []string{"0", "32001", "-5", "x", ""} {
		if _, err = parseArticleOptions("vn", "20990101", append(append([]string{}, base...), "--max-tokens", v)); err == nil {
			t.Fatalf("accepted --max-tokens %q", v)
		}
	}
}

func TestArticleRecipe_MaxTokensChangesHashAndReserve(t *testing.T) {
	doc, err := classify.NewRecipe("claude-sonnet-5-5", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	standard, err := classify.NewArticleRecipe(doc, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	capped, err := withArticleMaxTokens(standard, 1452)
	if err != nil {
		t.Fatal(err)
	}
	if capped.Hash() == standard.Hash() || capped.Article.MaxTokens != 1452 || standard.Article.MaxTokens != 8192 {
		t.Fatalf("standard %+v capped %+v", standard.Article, capped.Article)
	}
	if same, err := withArticleMaxTokens(standard, 0); err != nil || same.Hash() != standard.Hash() {
		t.Fatalf("zero must keep the default: %v", err)
	}
}
