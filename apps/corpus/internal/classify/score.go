package classify

import (
	"fmt"
	"math"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type Metrics struct {
	TruePositive      int     `json:"true_positive"`
	FalsePositive     int     `json:"false_positive"`
	FalseNegative     int     `json:"false_negative"`
	Precision         float64 `json:"precision"`
	Recall            float64 `json:"recall"`
	TopOnePrecision   float64 `json:"top_1_precision"`
	AdditionalCorrect int     `json:"additional_node_correct"`
	AdditionalTotal   int     `json:"additional_node_total"`
}

type ThresholdMetrics struct {
	Threshold float64 `json:"threshold"`
	Metrics   Metrics `json:"metrics"`
}

type ScoreBlock struct {
	Raw        Metrics            `json:"raw"`
	Candidates []ThresholdMetrics `json:"candidates"`
	Threshold  *float64           `json:"threshold"`
}

type LevelScore struct {
	Level       string     `json:"level"`
	All         ScoreBlock `json:"all"`
	NonFragment ScoreBlock `json:"non_fragment"`
}

type FragmentEntry struct {
	DocumentKey string `json:"document_key"`
	Anchor      string `json:"anchor"`
}

type ScoreReport struct {
	Status             string          `json:"status"`
	RecipeHash         string          `json:"recipe_hash"`
	LabelsHash         string          `json:"labels_sha256"`
	FamiliesHash       string          `json:"families_sha256"`
	Entries            int             `json:"entries"`
	InvalidEntries     int             `json:"invalid_entries"`
	FragmentCount      int             `json:"fragment_count"`
	NonFragmentEntries int             `json:"non_fragment_entries"`
	ExcludedFragments  []FragmentEntry `json:"excluded_fragments"`
	FamilyHits         int             `json:"family_hits"`
	FamilyTotal        int             `json:"family_total"`
	FamilyRecall       float64         `json:"family_recall"`
	Levels             []LevelScore    `json:"levels"`
}

type scoredArticle struct {
	truth         [3]map[string]bool
	predictions   [3]map[string]float64
	topCode       string
	topConfidence float64
	topMapped     [3]string
	noNode        bool
	confidences   [3][]float64
}

func Score(labels taxonomy.Labels, families FamilyArtifact, artifact ArticleArtifact, release []Article, nodes []Node) (ScoreReport, error) {
	report := ScoreReport{Status: "available", RecipeHash: artifact.RecipeHash, FamiliesHash: artifact.FamiliesHash, Entries: len(labels.Articles)}
	validation, err := validateReleaseLabels(labels, release, nodes)
	if err != nil {
		return report, err
	}
	report.InvalidEntries = validation.invalid
	byCode := map[string]Node{}
	for _, n := range nodes {
		byCode[n.Code] = n
	}
	predicted := map[string]ArticleOutcome{}
	for _, a := range artifact.Articles {
		key := a.DocumentKey + "/" + a.Anchor
		if _, ok := predicted[key]; ok {
			return report, fmt.Errorf("duplicate article prediction")
		}
		predicted[key] = a.Outcome
	}
	var scored, nonFragment []scoredArticle
	texts := map[string]string{}
	for _, article := range release {
		texts[article.DocumentKey+"/"+article.Anchor] = article.OwnText
	}
	report.ExcludedFragments = []FragmentEntry{}
	for _, label := range labels.Articles {
		key := label.Key + "/" + label.Anchor
		valid := validation.valid[key]
		truthCodes := map[string]bool{}
		for _, code := range label.Codes {
			truthCodes[code] = true
		}
		family := families.Documents[label.Key]
		for code := range truthCodes {
			report.FamilyTotal++
			if valid && family.Outcome == "ok" && containsCode(family.Codes, ancestor(code, 2, byCode)) {
				report.FamilyHits++
			}
		}
		item := scoredArticle{noNode: label.NoNode, truth: [3]map[string]bool{{}, {}, {}}, predictions: [3]map[string]float64{{}, {}, {}}}
		for code := range truthCodes {
			for i, level := range []int{2, 3, 0} {
				mapped := ancestor(code, level, byCode)
				if mapped == "" && !valid {
					mapped = "invalid/" + code
				}
				if mapped != "" {
					item.truth[i][mapped] = true
				}
			}
		}
		out := predicted[key]
		if valid && family.Outcome == "ok" && len(family.Codes) > 0 && out.Outcome == "ok" {
			for _, p := range out.Codes {
				n := byCode[p.Code]
				if !n.Active || n.Level < 2 || n.Level > 4 || (n.Origin != "phapdien" && n.Origin != "custom") || math.IsNaN(p.Confidence) || math.IsInf(p.Confidence, 0) || p.Confidence < 0 || p.Confidence > 1 || !containsCode(family.Codes, ancestor(p.Code, 2, byCode)) {
					return report, fmt.Errorf("invalid article prediction code or confidence")
				}
				if item.topCode == "" || p.Confidence > item.topConfidence || (p.Confidence == item.topConfidence && p.Code < item.topCode) {
					item.topCode, item.topConfidence = p.Code, p.Confidence
					for i, level := range []int{2, 3, 0} {
						item.topMapped[i] = ancestor(p.Code, level, byCode)
					}
				}
				for i, level := range []int{2, 3, 0} {
					mapped := ancestor(p.Code, level, byCode)
					if mapped == "" {
						continue
					}
					item.confidences[i] = append(item.confidences[i], p.Confidence)
					prior, ok := item.predictions[i][mapped]
					if !ok || p.Confidence > prior {
						item.predictions[i][mapped] = p.Confidence
					}
				}
			}
		}
		scored = append(scored, item)
		if text, present := texts[key]; present && taxonomy.IsFragment(text) {
			report.ExcludedFragments = append(report.ExcludedFragments, FragmentEntry{label.Key, label.Anchor})
		} else {
			nonFragment = append(nonFragment, item)
		}
	}
	if report.FamilyTotal > 0 {
		report.FamilyRecall = float64(report.FamilyHits) / float64(report.FamilyTotal)
	}
	availabilityErr := validation.availabilityError()
	unavailable := availabilityErr != nil
	report.FragmentCount = len(report.ExcludedFragments)
	report.NonFragmentEntries = len(nonFragment)
	for i, name := range []string{"level_2", "level_3", "deepest"} {
		report.Levels = append(report.Levels, LevelScore{Level: name, All: scoreBlock(scored, i, unavailable), NonFragment: scoreBlock(nonFragment, i, unavailable)})
	}

	if unavailable {
		report.Status = "unavailable"
		return report, availabilityErr
	}
	return report, nil
}

func scoreBlock(articles []scoredArticle, level int, unavailable bool) ScoreBlock {
	block := ScoreBlock{Raw: scoreAt(articles, level, 0)}
	candidates := map[float64]bool{0: true}
	for _, article := range articles {
		if article.noNode && level > 0 {
			continue
		}
		for _, confidence := range article.confidences[level] {
			candidates[confidence] = true
		}
	}
	values := make([]float64, 0, len(candidates))
	for value := range candidates {
		values = append(values, value)
	}
	sort.Float64s(values)
	for _, value := range values {
		metric := scoreAt(articles, level, value)
		block.Candidates = append(block.Candidates, ThresholdMetrics{value, metric})
		if !unavailable && block.Threshold == nil && metric.TopOnePrecision >= 0.85 {
			v := value
			block.Threshold = &v
		}
	}
	return block
}

func ancestor(code string, level int, nodes map[string]Node) string {
	if level == 0 {
		return code
	}
	for steps := 0; steps < 4 && code != ""; steps++ {
		n, ok := nodes[code]
		if !ok {
			return ""
		}
		if n.Level == level {
			return code
		}
		code = n.Parent
	}
	return ""
}

func containsCode(codes []string, code string) bool {
	if code == "" {
		return false
	}
	for _, v := range codes {
		if v == code {
			return true
		}
	}
	return false
}

func scoreAt(articles []scoredArticle, level int, threshold float64) Metrics {
	var m Metrics
	var topTotal, topCorrect int
	for _, a := range articles {
		if a.noNode && level > 0 {
			continue
		}
		if a.topCode != "" && a.topConfidence >= threshold {
			topTotal++
			if a.truth[level][a.topMapped[level]] {
				topCorrect++
			}
		}
		for code, confidence := range a.predictions[level] {
			if confidence < threshold {
				continue
			}
			if code != a.topMapped[level] {
				m.AdditionalTotal++
				if a.truth[level][code] {
					m.AdditionalCorrect++
				}
			}
			if a.truth[level][code] {
				m.TruePositive++
			} else {
				m.FalsePositive++
			}
		}
		for code := range a.truth[level] {
			if confidence, ok := a.predictions[level][code]; !ok || confidence < threshold {
				m.FalseNegative++
			}
		}
	}
	if topTotal > 0 {
		m.TopOnePrecision = float64(topCorrect) / float64(topTotal)
	}
	if m.TruePositive+m.FalsePositive > 0 {
		m.Precision = float64(m.TruePositive) / float64(m.TruePositive+m.FalsePositive)
	}
	if m.TruePositive+m.FalseNegative > 0 {
		m.Recall = float64(m.TruePositive) / float64(m.TruePositive+m.FalseNegative)
	}
	return m
}
