package classify

import (
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func scoreFixture(t *testing.T, n int) (taxonomy.Labels, FamilyArtifact, ArticleArtifact, []Article, []Node) {
	t.Helper()
	r := articleRecipe(t)
	nodes := articleNodes()
	nodes = append(nodes, Node{Code: "1.1.c1.m1", Parent: "1.1.c1", Level: 4, Active: true, Origin: "phapdien"})
	labels := taxonomy.Labels{Country: "vn", Release: "20990101"}
	release := articleInputs(n, "")
	families := FamilyArtifact{Recipe: r.documentRecipe(), RecipeHash: r.Article.SourceRecipeHash, TaxonomyHash: r.TaxonomyHash, Documents: map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}}
	artifact := ArticleArtifact{Recipe: r, RecipeHash: r.Hash(), TaxonomyHash: r.TaxonomyHash, FamiliesHash: r.Article.FamiliesHash}
	for _, a := range release {
		labels.Articles = append(labels.Articles, taxonomy.LabelArticle{Key: a.DocumentKey, Anchor: a.Anchor, OwnTextSHA256: taxonomy.OwnTextSHA256(a.OwnText), Codes: []string{"1.1.c1.m1"}})
		artifact.Articles = append(artifact.Articles, ArticleRecord{a.DocumentKey, a.Anchor, ArticleOutcome{Outcome: "ok", Codes: []CodeConfidence{{"1.1.c1.m1", 0.9}}}})
	}
	return labels, families, artifact, release, nodes
}

func TestScore_MicroSetsAncestorsAndLowestThreshold(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 2)
	l.Articles[0].Codes = []string{"1.1.c1.m1", "1.1.x1"}
	a.Articles[0].Outcome.Codes = []CodeConfidence{{"1.1.c1.m1", 0.9}, {"1.1.c1", 0.7}, {"1.1.x1", 0.8}}
	a.Articles[1].Outcome.Codes = append(a.Articles[1].Outcome.Codes, CodeConfidence{"1.1.x1", 0.6})
	report, err := Score(l, f, a, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	wants := []struct {
		tp, fp, fn int
		threshold  float64
	}{{2, 0, 0, 0}, {3, 1, 0, 0}, {3, 2, 0, 0}}
	for i, want := range wants {
		row := report.Levels[i]
		if row.Raw.TruePositive != want.tp || row.Raw.FalsePositive != want.fp || row.Raw.FalseNegative != want.fn || row.Threshold == nil || *row.Threshold != want.threshold {
			t.Fatalf("row %+v want %+v", row, want)
		}
	}
	if report.FamilyRecall != 1 {
		t.Fatalf("family recall %f", report.FamilyRecall)
	}
}

func TestScore_StaleLabelsNeverShrinkDenominator(t *testing.T) {
	for _, count := range []int{20, 19} {
		l, f, a, release, nodes := scoreFixture(t, count)
		l.Articles[0].OwnTextSHA256 = "stale"
		report, err := Score(l, f, a, release, nodes)
		if (err != nil) != (count == 19) || report.InvalidEntries != 1 || report.Levels[2].Raw.FalseNegative != 1 || report.FamilyRecall != float64(count-1)/float64(count) {
			t.Fatalf("count %d report %+v error %v", count, report, err)
		}
		if count == 19 {
			for _, level := range report.Levels {
				if level.Threshold != nil {
					t.Fatal("unavailable set calibrated a threshold")
				}
			}
		}
	}
}

func TestScore_FailedFamiliesAndInvalidLabelsAreMisses(t *testing.T) {
	for _, reason := range []string{"missing document", "missing anchor", "retired code", "unknown code", "empty family", "failed family"} {
		t.Run(reason, func(t *testing.T) {
			l, f, a, release, nodes := scoreFixture(t, 20)
			switch reason {
			case "missing document":
				l.Articles[0].Key = "missing"
			case "missing anchor":
				l.Articles[0].Anchor = "missing"
			case "retired code":
				l.Articles[0].Codes = []string{"1.1.old"}
			case "unknown code":
				l.Articles[0].Codes = []string{"unknown"}
			case "empty family":
				f.Documents["vn:vbpl:1"] = Outcome{Outcome: "no_family"}
			case "failed family":
				f.Documents["vn:vbpl:1"] = Outcome{Outcome: "invalid_response"}
			}
			report, err := Score(l, f, a, release, nodes)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if reason == "empty family" || reason == "failed family" {
				want = 20
			}
			if report.Levels[2].Raw.FalseNegative != want {
				t.Fatalf("report %+v", report)
			}
		})
	}
}

func TestScore_NoQualifyingThresholdAndDuplicateLabels(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 1)
	a.Articles[0].Outcome.Codes = []CodeConfidence{{"1.1.x1", 0.9}}
	report, err := Score(l, f, a, release, nodes)
	if err != nil || report.Levels[2].Threshold != nil {
		t.Fatalf("report %+v error %v", report, err)
	}
	l.Articles = append(l.Articles, l.Articles[0])
	if _, err = Score(l, f, a, release, nodes); err == nil {
		t.Fatal("accepted duplicate labeled article")
	}
}

func TestScore_TopOneAndThreshold(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 3)
	a.Articles[0].Outcome.Codes = []CodeConfidence{{"1.1.x1", 0.8}, {"1.1.c1.m1", 0.8}}
	a.Articles[1].Outcome.Codes = []CodeConfidence{{"1.1.x1", 0.9}, {"1.1.c1.m1", 0.7}}
	a.Articles[2].Outcome.Codes = nil
	report, err := Score(l, f, a, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []float64{1, 0.5, 0.5} {
		b, marshalErr := json.Marshal(report.Levels[i].Raw)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		var m struct {
			TopOne float64 `json:"top_1_precision"`
		}
		if err = json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if m.TopOne != want {
			t.Errorf("level %d top-1 %v want %v", i, m.TopOne, want)
		}
	}
	if report.Levels[2].Threshold != nil {
		t.Errorf("incorrect threshold %v", *report.Levels[2].Threshold)
	}
	a.Articles[1].Outcome.Codes[0].Confidence = 0.6
	a.Articles[1].Outcome.Codes[1].Confidence = 0.5
	report, err = Score(l, f, a, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if report.Levels[2].Threshold == nil || *report.Levels[2].Threshold != 0.8 {
		t.Errorf("threshold %+v", report.Levels[2].Threshold)
	}
}
