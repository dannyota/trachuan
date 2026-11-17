package classify

import (
	"fmt"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type CalibrationSet struct {
	Labels                                 taxonomy.Labels
	Families                               FamilyArtifact
	Articles                               ArticleArtifact
	Release                                []Article
	Nodes                                  []Node
	LabelsHash, FamiliesHash, ArticlesHash string
}

type CalibrationMetrics struct {
	SeededAll              Metrics           `json:"seeded_all"`
	SeededNonFragment      Metrics           `json:"seeded_non_fragment"`
	Adjudicated            []Metrics         `json:"adjudicated"`
	AdjudicatedNonFragment []Metrics         `json:"adjudicated_non_fragment"`
	SeededFragments        []FragmentEntry   `json:"seeded_fragments"`
	AdjudicatedFragments   [][]FragmentEntry `json:"adjudicated_fragments"`
	AdditionalCorrect      int               `json:"additional_correct"`
	AdditionalTotal        int               `json:"additional_total"`
}

type CalibrationResult struct {
	Direction           string             `json:"direction,omitempty"`
	PrimaryThreshold    *float64           `json:"primary_threshold"`
	AdditionalThreshold *float64           `json:"additional_threshold"`
	Metrics             CalibrationMetrics `json:"metrics"`
}

type CalibrationBinding struct {
	RecipeHash   string `json:"recipe_hash"`
	LabelsHash   string `json:"labels_sha256"`
	FamiliesHash string `json:"families_sha256"`
	ArticlesHash string `json:"articles_sha256"`
}

type CalibrationReport struct {
	CalibrationResult
	Bindings  []CalibrationBinding `json:"bindings"`
	SplitRule string               `json:"split_rule"`
	Split     []CalibrationResult  `json:"split"`
}

func Calibrate(seed CalibrationSet, adjudicated []CalibrationSet) (CalibrationReport, error) {
	var report CalibrationReport
	if len(adjudicated) == 0 {
		return report, fmt.Errorf("calibration requires an adjudicated set")
	}
	sets := append([]CalibrationSet{seed}, adjudicated...)
	var commonRecipe string
	for _, s := range sets {
		if s.Articles.Recipe.Article == nil || s.Articles.Recipe.Article.PromptVersion != 4 || s.Articles.Recipe.Article.Step != 0 {
			return report, fmt.Errorf("calibration requires menu article recipes")
		}
		validation, err := validateReleaseLabels(s.Labels, s.Release, s.Nodes)
		if err != nil {
			return report, err
		}
		if err = validation.availabilityError(); err != nil {
			return report, err
		}
		r := s.Articles.Recipe
		article := *r.Article
		article.FamiliesHash = ""
		r.Article = &article
		if commonRecipe != "" && commonRecipe != r.Hash() {
			return report, fmt.Errorf("calibration recipes differ")
		}
		commonRecipe = r.Hash()
		report.Bindings = append(report.Bindings, CalibrationBinding{s.Articles.RecipeHash, s.LabelsHash, s.FamiliesHash, s.ArticlesHash})
	}
	var err error
	report.CalibrationResult, err = calibrateSets(seed, adjudicated)
	if err != nil {
		return report, err
	}
	report.SplitRule = "Sort each set by SHA-256 of key/anchor; the first floor(n/2) entries are A and the rest B."
	a, b := make([]CalibrationSet, len(sets)), make([]CalibrationSet, len(sets))
	for i, s := range sets {
		a[i], b[i] = splitCalibrationSet(s)
	}
	for index, pair := range [][2][]CalibrationSet{{a, b}, {b, a}} {
		result, e := calibrateSets(pair[0][0], pair[0][1:])
		if e != nil {
			return report, e
		}
		if index == 0 {
			result.Direction = "train_a_test_b"
		} else {
			result.Direction = "train_b_test_a"
		}
		result.Metrics, e = calibrationMetrics(pair[1][0], pair[1][1:], result.PrimaryThreshold, result.AdditionalThreshold)
		if e != nil {
			return report, e
		}
		report.Split = append(report.Split, result)
	}
	return report, nil
}

func calibrateSets(seed CalibrationSet, adjudicated []CalibrationSet) (CalibrationResult, error) {
	var result CalibrationResult
	for _, threshold := range calibrationCandidates([]CalibrationSet{seed}, 0, nil) {
		score, err := calibrationScore(seed, &threshold, nil)
		if err != nil {
			return result, err
		}
		m := score.Levels[0].NonFragment.Raw
		if m.TopOnePrecision >= 0.9 {
			v := threshold
			result.PrimaryThreshold = &v
			break
		}
	}
	for _, threshold := range calibrationCandidates(adjudicated, 1, result.PrimaryThreshold) {
		var correct, total int
		for _, s := range adjudicated {
			score, err := calibrationScore(s, result.PrimaryThreshold, &threshold)
			if err != nil {
				return result, err
			}
			m := score.Levels[0].All.Raw
			correct += m.AdditionalCorrect
			total += m.AdditionalTotal
		}
		if total > 0 && float64(correct)/float64(total) >= 0.8 {
			v := threshold
			result.AdditionalThreshold = &v
			break
		}
	}
	var err error
	result.Metrics, err = calibrationMetrics(seed, adjudicated, result.PrimaryThreshold, result.AdditionalThreshold)
	return result, err
}

func calibrationCandidates(sets []CalibrationSet, index int, primary *float64) []float64 {
	seen := map[float64]bool{}
	for _, s := range sets {
		valid, err := validateReleaseLabels(s.Labels, s.Release, s.Nodes)
		if err != nil {
			continue
		}
		eligible := map[string]bool{}
		for key, ok := range valid.valid {
			eligible[key] = ok
		}
		if index == 0 {
			for _, a := range s.Release {
				if taxonomy.IsFragment(a.OwnText) {
					delete(eligible, a.DocumentKey+"/"+a.Anchor)
				}
			}
		}
		for _, a := range s.Articles.Articles {
			if eligible[a.DocumentKey+"/"+a.Anchor] && a.Outcome.Outcome == "ok" && len(a.Outcome.Codes) > index && (index == 0 || (primary != nil && a.Outcome.Codes[0].Confidence >= *primary)) {
				seen[a.Outcome.Codes[index].Confidence] = true
			}
		}
	}
	values := make([]float64, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Float64s(values)
	return values
}

func calibrationScore(s CalibrationSet, primary, additional *float64) (ScoreReport, error) {
	a := s.Articles
	a.Articles = append([]ArticleRecord(nil), a.Articles...)
	for i := range a.Articles {
		out := a.Articles[i].Outcome
		if out.Outcome != "ok" {
			continue
		}
		if len(out.Codes) == 0 || primary == nil || out.Codes[0].Confidence < *primary {
			out = emptyArticle("invalid_response")
		} else {
			out.Codes = append([]CodeConfidence(nil), out.Codes...)
			if len(out.Codes) > 1 && (additional == nil || out.Codes[1].Confidence < *additional) {
				out.Codes = out.Codes[:1]
			}
		}
		a.Articles[i].Outcome = out
	}
	report, err := Score(s.Labels, s.Families, a, s.Release, s.Nodes)
	if report.Status == "unavailable" {
		return report, nil
	}
	return report, err
}

func calibrationMetrics(seed CalibrationSet, adjudicated []CalibrationSet, primary, additional *float64) (CalibrationMetrics, error) {
	var m CalibrationMetrics
	score, err := calibrationScore(seed, primary, additional)
	if err != nil {
		return m, err
	}
	m.SeededAll, m.SeededNonFragment = score.Levels[0].All.Raw, score.Levels[0].NonFragment.Raw
	m.SeededFragments = score.ExcludedFragments
	for _, s := range adjudicated {
		score, err = calibrationScore(s, primary, additional)
		if err != nil {
			return m, err
		}
		v := score.Levels[0].All.Raw
		m.Adjudicated = append(m.Adjudicated, v)
		m.AdjudicatedNonFragment = append(m.AdjudicatedNonFragment, score.Levels[0].NonFragment.Raw)
		m.AdjudicatedFragments = append(m.AdjudicatedFragments, score.ExcludedFragments)
		m.AdditionalCorrect += v.AdditionalCorrect
		m.AdditionalTotal += v.AdditionalTotal
	}
	return m, nil
}
