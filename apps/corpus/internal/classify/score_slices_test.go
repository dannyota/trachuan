package classify

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestScore_FragmentSlices(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 2)
	release[0].OwnText = "Điều 1. Heading\nThông tư này có hiệu lực từ ngày 1 tháng 1 năm 2027."
	l.Articles[0].OwnTextSHA256 = taxonomy.OwnTextSHA256(release[0].OwnText)
	a.Articles[0].Outcome.Codes = nil
	report, err := Score(l, f, a, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		FragmentCount      int `json:"fragment_count"`
		NonFragmentEntries int `json:"non_fragment_entries"`
		Excluded           []struct {
			Key    string `json:"document_key"`
			Anchor string `json:"anchor"`
		} `json:"excluded_fragments"`
		Levels []struct {
			All struct {
				Raw Metrics `json:"raw"`
			} `json:"all"`
			NonFragment struct {
				Raw Metrics `json:"raw"`
			} `json:"non_fragment"`
		} `json:"levels"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.NonFragmentEntries != 1 || got.FragmentCount != 1 || len(got.Excluded) != 1 || got.Excluded[0].Key != release[0].DocumentKey || got.Excluded[0].Anchor != release[0].Anchor {
		t.Fatalf("fragment details: %s", b)
	}
	for _, level := range got.Levels {
		if level.All.Raw.Recall != 0.5 || level.NonFragment.Raw.Recall != 1 {
			t.Fatalf("scores: %s", b)
		}
	}
}

func TestScore_AdditionalNodesAndNoNode(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 2)
	l.Articles[0].Codes = []string{"1.1.c1.m1", "1.1.x1"}
	nodes = append(nodes, Node{Code: "1.1.c2", Parent: "1.1", Level: 3, Active: true, Origin: "phapdien"})
	a.Articles[0].Outcome.Codes = []CodeConfidence{{"1.1.c1.m1", 0.9}, {"1.1.x1", 0.8}, {"1.1.c2", 0.7}}
	l.Articles[1].Codes = []string{"1.1"}
	l.Articles[1].NoNode = true
	a.Articles[1].Outcome.Codes = []CodeConfidence{{"1.1.c2", 0.9}}
	report, err := Score(l, f, a, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Levels []struct {
			All struct {
				Raw struct {
					TruePositive      int `json:"true_positive"`
					FalsePositive     int `json:"false_positive"`
					AdditionalCorrect int `json:"additional_node_correct"`
					AdditionalTotal   int `json:"additional_node_total"`
				} `json:"raw"`
			} `json:"all"`
		} `json:"levels"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{1, 2} {
		m := got.Levels[i].All.Raw
		if m.TruePositive != 2 || m.FalsePositive != 1 || m.AdditionalCorrect != 1 || m.AdditionalTotal != 2 {
			t.Fatalf("level %d: %s", i, b)
		}
	}
}

func TestScore_ThresholdCandidatesIncludeMappedDuplicates(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 1)
	a.Articles[0].Outcome.Codes = []CodeConfidence{{"1.1.c1.m1", 0.9}, {"1.1.c1", 0.7}}
	report, err := Score(l, f, a, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range report.Levels[0].All.Candidates {
		if candidate.Threshold == 0.7 {
			found = true
		}
	}
	if !found {
		t.Fatal("missing threshold for a prediction mapped to the same node")
	}
}

func TestScore_HistoricalArtifacts(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			labels, families, a, release, nodes := scoreFixture(t, 1)
			if version == 1 {
				a.Recipe.PromptVersion = 1
				a.Recipe.Article.SourceRecipeHash = a.Recipe.documentRecipe().Hash()
			}
			if version == 3 {
				a.Recipe = stepRecipe(t, 2)
				zero := 0
				a.Articles[0].Outcome.FailedFamilies = &zero
			}
			a.Version = 1
			a.RecipeHash = a.Recipe.Hash()
			a.TaxonomyHash = a.Recipe.TaxonomyHash
			a.FamiliesHash = a.Recipe.Article.FamiliesHash
			b, err := json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "articles.json")
			if err = writeOnce(path, b); err != nil {
				t.Fatal(err)
			}
			if err = writeOnce(filepath.Join(dir, "MANIFEST.sha256"), []byte(digest(b)+"  articles.json\n")); err != nil {
				t.Fatal(err)
			}
			read, err := ReadArticleArtifactForScore(path, a.TaxonomyHash, a.FamiliesHash)
			if err != nil {
				t.Fatal(err)
			}
			report, err := Score(labels, families, read, release, nodes)
			if err != nil {
				t.Fatal(err)
			}
			if report.Levels[2].All.Raw.Recall != 1 || report.Levels[2].NonFragment.Raw.Recall != 1 {
				t.Fatalf("report %+v", report)
			}
		})
	}
}
