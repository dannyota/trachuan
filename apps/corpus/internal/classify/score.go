package classify

import (
	"fmt"
	"math"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type Metrics struct {
	TruePositive  int     `json:"true_positive"`
	FalsePositive int     `json:"false_positive"`
	FalseNegative int     `json:"false_negative"`
	Precision     float64 `json:"precision"`
	Recall        float64 `json:"recall"`
}

type ThresholdMetrics struct {
	Threshold float64 `json:"threshold"`
	Metrics   Metrics `json:"metrics"`
}

type LevelScore struct {
	Level      string             `json:"level"`
	Raw        Metrics            `json:"raw"`
	Candidates []ThresholdMetrics `json:"candidates"`
	Threshold  *float64           `json:"threshold"`
}

type ScoreReport struct {
	Status         string       `json:"status"`
	RecipeHash     string       `json:"recipe_hash"`
	LabelsHash     string       `json:"labels_sha256"`
	FamiliesHash   string       `json:"families_sha256"`
	Entries        int          `json:"entries"`
	InvalidEntries int          `json:"invalid_entries"`
	FamilyHits     int          `json:"family_hits"`
	FamilyTotal    int          `json:"family_total"`
	FamilyRecall   float64      `json:"family_recall"`
	Levels         []LevelScore `json:"levels"`
}

type scoredArticle struct {
	truth       [3]map[string]bool
	predictions [3]map[string]float64
}

func Score(labels taxonomy.Labels, families FamilyArtifact, artifact ArticleArtifact, release []Article, nodes []Node) (ScoreReport, error) {
	report := ScoreReport{Status: "available", RecipeHash: artifact.RecipeHash, FamiliesHash: artifact.FamiliesHash, Entries: len(labels.Articles)}
	if len(labels.Articles) == 0 {
		return report, fmt.Errorf("labeled set is empty")
	}
	byCode := map[string]Node{}
	for _, n := range nodes {
		byCode[n.Code] = n
	}
	texts := map[string]string{}
	for _, a := range release {
		texts[a.DocumentKey+"/"+a.Anchor] = a.OwnText
	}
	predicted := map[string]ArticleOutcome{}
	for _, a := range artifact.Articles {
		key := a.DocumentKey + "/" + a.Anchor
		if _, ok := predicted[key]; ok {
			return report, fmt.Errorf("duplicate article prediction")
		}
		predicted[key] = a.Outcome
	}
	seen := map[string]bool{}
	var scored []scoredArticle
	candidates := [3]map[float64]bool{{0: true}, {0: true}, {0: true}}
	for _, label := range labels.Articles {
		key := label.Key + "/" + label.Anchor
		if seen[key] || label.Key == "" || label.Anchor == "" || len(label.Codes) == 0 {
			return report, fmt.Errorf("invalid or duplicate labeled article")
		}
		seen[key] = true
		text, present := texts[key]
		valid := present && taxonomy.OwnTextSHA256(text) == label.OwnTextSHA256
		truthCodes := map[string]bool{}
		for _, code := range label.Codes {
			n, ok := byCode[code]
			if !ok || !n.Active {
				valid = false
			}
			truthCodes[code] = true
		}
		if !valid {
			report.InvalidEntries++
		}
		family := families.Documents[label.Key]
		for code := range truthCodes {
			report.FamilyTotal++
			if valid && family.Outcome == "ok" && containsCode(family.Codes, ancestor(code, 2, byCode)) {
				report.FamilyHits++
			}
		}
		item := scoredArticle{truth: [3]map[string]bool{{}, {}, {}}, predictions: [3]map[string]float64{{}, {}, {}}}
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
				for i, level := range []int{2, 3, 0} {
					mapped := ancestor(p.Code, level, byCode)
					if mapped == "" {
						continue
					}
					candidates[i][p.Confidence] = true
					prior, ok := item.predictions[i][mapped]
					if !ok || p.Confidence > prior {
						item.predictions[i][mapped] = p.Confidence
					}
				}
			}
		}
		scored = append(scored, item)
	}
	if report.FamilyTotal > 0 {
		report.FamilyRecall = float64(report.FamilyHits) / float64(report.FamilyTotal)
	}
	unavailable := report.InvalidEntries*20 > report.Entries
	for i, name := range []string{"level_2", "level_3", "deepest"} {
		row := LevelScore{Level: name, Raw: scoreAt(scored, i, 0)}
		values := make([]float64, 0, len(candidates[i]))
		for value := range candidates[i] {
			values = append(values, value)
		}
		sort.Float64s(values)
		for _, value := range values {
			metric := scoreAt(scored, i, value)
			row.Candidates = append(row.Candidates, ThresholdMetrics{value, metric})
			if !unavailable && row.Threshold == nil && metric.TruePositive+metric.FalsePositive > 0 && metric.Precision >= 0.85 {
				v := value
				row.Threshold = &v
			}
		}
		report.Levels = append(report.Levels, row)
	}
	if unavailable {
		report.Status = "unavailable"
		return report, fmt.Errorf("more than 5 percent of labeled entries fail release validation")
	}
	return report, nil
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
	for _, a := range articles {
		for code, confidence := range a.predictions[level] {
			if confidence < threshold {
				continue
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
	if m.TruePositive+m.FalsePositive > 0 {
		m.Precision = float64(m.TruePositive) / float64(m.TruePositive+m.FalsePositive)
	}
	if m.TruePositive+m.FalseNegative > 0 {
		m.Recall = float64(m.TruePositive) / float64(m.TruePositive+m.FalseNegative)
	}
	return m
}
