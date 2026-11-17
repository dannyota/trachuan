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

func TestClassifyCalibrate_RequiresInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"classify", "calibrate"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "classify calibrate") {
		t.Fatalf("code %d stderr %q", code, stderr.String())
	}
}

func TestClassifyCalibrate_WritesBoundReport(t *testing.T) {
	dir := t.TempDir()
	taxHash := strings.Repeat("a", 64)
	labels := taxonomy.Labels{Country: "vn", Release: "20990101"}
	evidence := classificationEvidence{taxonomyHash: taxHash, nodes: []classify.Node{{Code: "1.1", Level: 2, Active: true, Origin: "phapdien"}}}
	for i := 0; i < 8; i++ {
		anchor := fmt.Sprintf("dieu-%d", i)
		text := "Synthetic operative text"
		labels.Articles = append(labels.Articles, taxonomy.LabelArticle{Key: "vn:vbpl:1", Anchor: anchor, Codes: []string{"1.1"}, OwnTextSHA256: taxonomy.OwnTextSHA256(text)})
		evidence.articles = append(evidence.articles, classify.Article{DocumentKey: "vn:vbpl:1", Anchor: anchor, OwnText: text})
	}
	write := func(name string, value any, manifest bool) string {
		t.Helper()
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(d, name+".json")
		if err = os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		if manifest {
			if err = os.WriteFile(filepath.Join(d, "MANIFEST.sha256"), fmt.Appendf(nil, "%x  %s.json\n", sha256.Sum256(b), name), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	labelsPath := write("labels", labels, false)
	labelsBytes, err := os.ReadFile(labelsPath)
	if err != nil {
		t.Fatal(err)
	}
	labelsHash := fmt.Sprintf("%x", sha256.Sum256(labelsBytes))
	r, err := classify.NewRecipe("claude-haiku-5-5", taxHash)
	if err != nil {
		t.Fatal(err)
	}
	f := classify.FamilyArtifact{Version: 1, Recipe: r, RecipeHash: r.Hash(), TaxonomyHash: taxHash, Documents: map[string]classify.Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}}
	familyPath := write("families", f, true)
	familyBytes, err := os.ReadFile(familyPath)
	if err != nil {
		t.Fatal(err)
	}
	familyHash := fmt.Sprintf("%x", sha256.Sum256(familyBytes))
	ar, err := classify.NewArticleRecipe(r, familyHash)
	if err != nil {
		t.Fatal(err)
	}
	a := classify.ArticleArtifact{Version: 1, Country: "vn", Release: "20990101", Recipe: ar, RecipeHash: ar.Hash(), TaxonomyHash: taxHash, FamiliesHash: familyHash, LabelsHash: labelsHash}
	for _, v := range evidence.articles {
		a.Articles = append(a.Articles, classify.ArticleRecord{DocumentKey: v.DocumentKey, Anchor: v.Anchor, Outcome: classify.ArticleOutcome{Outcome: "ok", Codes: []classify.CodeConfidence{{Code: "1.1", Confidence: 0.9}}}})
	}
	articlePath := write("articles", a, true)
	triple := labelsPath + ":" + familyPath + ":" + articlePath
	out := filepath.Join(dir, "out")
	var stdout, stderr bytes.Buffer
	load := func(context.Context, string, string, taxonomy.Labels) (classificationEvidence, error) {
		return evidence, nil
	}
	code := runClassifyCalibrateWith(context.Background(), []string{"vn", "20990101", "--seeded", triple, "--adjudicated", triple, "--out", out}, &stdout, &stderr, load)
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	for _, name := range []string{"calibration.json", "MANIFEST.sha256"} {
		if _, err = os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatal(err)
		}
	}
}
