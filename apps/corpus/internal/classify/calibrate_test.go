package classify

import (
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestCalibrate_ThresholdsAndSplit(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 4)
	nodes = append(nodes, Node{Code: "2.1", Level: 2, Active: true, Origin: "phapdien"})
	a.Recipe.Article.PromptVersion = 4
	for i := range a.Articles {
		a.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}, {"2.1", 0.5}}
	}
	a.Articles[0].Outcome.Codes[0] = CodeConfidence{"2.1", 0.6}
	a.Articles[0].Outcome.Codes[1] = CodeConfidence{"1.1", 0.8}
	s := CalibrationSet{Labels: l, Families: f, Articles: a, Release: release, Nodes: nodes}
	r, err := Calibrate(s, []CalibrationSet{s})
	if err != nil {
		t.Fatal(err)
	}
	if r.PrimaryThreshold == nil || *r.PrimaryThreshold != 0.9 || r.AdditionalThreshold != nil {
		t.Fatalf("thresholds %+v", r)
	}
	if len(r.Split) != 2 {
		t.Fatalf("split %+v", r.Split)
	}
}

func TestCalibrate_AdditionalThresholdExcludesRejectedPrimaries(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 20)
	nodes = append(nodes, Node{Code: "2.1", Level: 2, Active: true, Origin: "phapdien"})
	a.Recipe.Article.PromptVersion = 4
	seed := CalibrationSet{Labels: l, Families: f, Articles: a, Release: release, Nodes: nodes}
	seed.Articles.Articles = append([]ArticleRecord(nil), a.Articles...)
	adj := seed
	adj.Articles.Articles = append([]ArticleRecord(nil), a.Articles...)
	halfA, halfB := splitCalibrationSet(seed)
	badSeed, badPrimary := map[string]bool{}, map[string]bool{}
	for _, half := range []CalibrationSet{halfA, halfB} {
		for i, label := range half.Labels.Articles {
			if i < 2 {
				badSeed[label.Anchor] = true
			}
			if i < 8 {
				badPrimary[label.Anchor] = true
			}
		}
	}
	for i := range seed.Articles.Articles {
		anchor := seed.Articles.Articles[i].Anchor
		seed.Articles.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}}
		if badSeed[anchor] {
			seed.Articles.Articles[i].Outcome.Codes[0] = CodeConfidence{"2.1", 0.6}
		}
		adj.Articles.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}, {"2.1", 0.8}}
		if badPrimary[anchor] {
			adj.Articles.Articles[i].Outcome.Codes = []CodeConfidence{{"2.1", 0.6}, {"1.1", 0.2}}
		}
	}
	r, err := Calibrate(seed, []CalibrationSet{adj})
	if err != nil {
		t.Fatal(err)
	}
	if r.PrimaryThreshold == nil || *r.PrimaryThreshold != 0.9 || r.AdditionalThreshold != nil {
		t.Fatalf("full calibration %+v", r.CalibrationResult)
	}
	for _, split := range r.Split {
		if split.PrimaryThreshold == nil || *split.PrimaryThreshold != 0.9 || split.AdditionalThreshold != nil {
			t.Fatalf("split calibration %+v", split)
		}
	}
}

func TestCalibrate_ListsFragments(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 4)
	a.Recipe.Article.PromptVersion = 4
	for i := range a.Articles {
		a.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}}
	}
	release[0].OwnText = "Điều 1. Heading\nThông tư này có hiệu lực từ ngày 1 tháng 1 năm 2027."
	l.Articles[0].OwnTextSHA256 = taxonomy.OwnTextSHA256(release[0].OwnText)
	s := CalibrationSet{Labels: l, Families: f, Articles: a, Release: release, Nodes: nodes}
	r, err := Calibrate(s, []CalibrationSet{s})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r.Metrics)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Seeded      []FragmentEntry   `json:"seeded_fragments"`
		Adjudicated [][]FragmentEntry `json:"adjudicated_fragments"`
	}
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Seeded) != 1 || len(got.Adjudicated) != 1 || len(got.Adjudicated[0]) != 1 {
		t.Fatalf("metrics %s", b)
	}
}

func TestCalibrate_NoAdditionalThresholdAndStableHalves(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 8)
	nodes = append(nodes, Node{Code: "2.1", Level: 2, Active: true, Origin: "phapdien"})
	a.Recipe.Article.PromptVersion = 4
	for i := range a.Articles {
		a.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}, {"2.1", 0.7}}
	}
	s := CalibrationSet{Labels: l, Families: f, Articles: a, Release: release, Nodes: nodes}
	first, err := Calibrate(s, []CalibrationSet{s})
	if err != nil {
		t.Fatal(err)
	}
	if first.AdditionalThreshold != nil || first.Split[0].Direction != "train_a_test_b" || first.Split[1].Direction != "train_b_test_a" {
		t.Fatalf("report %+v", first)
	}
	for i, j := 0, len(s.Labels.Articles)-1; i < j; i, j = i+1, j-1 {
		s.Labels.Articles[i], s.Labels.Articles[j] = s.Labels.Articles[j], s.Labels.Articles[i]
	}
	second, err := Calibrate(s, []CalibrationSet{s})
	if err != nil {
		t.Fatal(err)
	}
	x, _ := json.Marshal(first)
	y, _ := json.Marshal(second)
	if string(x) != string(y) {
		t.Fatalf("split changed on reorder: %s != %s", x, y)
	}
}

func TestCalibrate_ExactPrecisionBoundaries(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 10)
	nodes = append(nodes, Node{Code: "2.1", Level: 2, Active: true, Origin: "phapdien"})
	a.Recipe.Article.PromptVersion = 4
	for i := range a.Articles {
		a.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}}
	}
	a.Articles[0].Outcome.Codes[0] = CodeConfidence{"2.1", 0.9}
	seed := CalibrationSet{Labels: l, Families: f, Articles: a, Release: release, Nodes: nodes}
	adj := seed
	adj.Labels.Articles = append(adj.Labels.Articles[:0:0], adj.Labels.Articles...)
	adj.Articles.Articles = append(adj.Articles.Articles[:0:0], adj.Articles.Articles...)
	for i := 0; i < 5; i++ {
		adj.Articles.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}, {"2.1", 0.8}}
		if i < 4 {
			adj.Labels.Articles[i].Codes = append(append([]string(nil), adj.Labels.Articles[i].Codes...), "2.1")
		}
	}
	r, err := Calibrate(seed, []CalibrationSet{adj})
	if err != nil {
		t.Fatal(err)
	}
	if r.PrimaryThreshold == nil || *r.PrimaryThreshold != 0.9 || r.AdditionalThreshold == nil || *r.AdditionalThreshold != 0.8 {
		t.Fatalf("report %+v", r)
	}
	if r.Metrics.SeededNonFragment.TopOnePrecision != 0.9 || r.Metrics.AdditionalCorrect != 4 || r.Metrics.AdditionalTotal != 5 {
		t.Fatalf("metrics %+v", r.Metrics)
	}
}

func TestCalibrate_NoPrimaryThreshold(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 4)
	nodes = append(nodes, Node{Code: "2.1", Level: 2, Active: true, Origin: "phapdien"})
	a.Recipe.Article.PromptVersion = 4
	for i := range a.Articles {
		a.Articles[i].Outcome.Codes = []CodeConfidence{{"2.1", 0.9}}
	}
	s := CalibrationSet{Labels: l, Families: f, Articles: a, Release: release, Nodes: nodes}
	r, err := Calibrate(s, []CalibrationSet{s})
	if err != nil {
		t.Fatal(err)
	}
	if r.PrimaryThreshold != nil || r.AdditionalThreshold != nil || r.Metrics.SeededAll.Recall != 0 {
		t.Fatalf("report %+v", r)
	}
}

func TestCalibrate_SplitDoesNotApplyAvailabilityToHalf(t *testing.T) {
	l, f, a, release, nodes := scoreFixture(t, 40)
	a.Recipe.Article.PromptVersion = 4
	for i := range a.Articles {
		a.Articles[i].Outcome.Codes = []CodeConfidence{{"1.1", 0.9}}
	}
	s := CalibrationSet{Labels: l, Families: f, Articles: a, Release: release, Nodes: nodes}
	half, _ := splitCalibrationSet(s)
	bad := map[string]bool{half.Labels.Articles[0].Anchor: true, half.Labels.Articles[1].Anchor: true}
	for i := range s.Labels.Articles {
		if bad[s.Labels.Articles[i].Anchor] {
			s.Labels.Articles[i].OwnTextSHA256 = "stale"
		}
	}
	if _, err := Calibrate(s, []CalibrationSet{s}); err != nil {
		t.Fatal(err)
	}
}
