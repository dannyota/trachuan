package evaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

type Revision struct {
	Cases   int           `json:"cases"`
	Value   *float64      `json:"value"`
	Revised []RevisedCase `json:"revised"`
}

type RevisedCase struct {
	CaseID string `json:"case_id"`
	Reason string `json:"reason"`
}

type developmentRevision struct {
	comparable, previousOrder []string
	revised                   []RevisedCase
}

func loadPreviousDevelopment(suite Suite, hash string) ([]Case, error) {
	data, ok := suite.PreviousDevelopment[hash]
	if !ok {
		return nil, fmt.Errorf("previous development fixture %s not found", hash)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != hash {
		return nil, fmt.Errorf("previous development fixture %s has a different SHA-256", hash)
	}
	cases, err := LoadDevelopment(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("previous development fixture %s: %w", hash, err)
	}
	return cases, nil
}

func previousCohorts(previous *Report) (map[string]string, error) {
	out := map[string]string{}
	for _, c := range previous.Cases {
		if c.Cohort != "development" && c.Cohort != "development_period" {
			continue
		}
		if cohort, ok := out[c.CaseID]; ok && cohort != c.Cohort {
			return nil, fmt.Errorf("previous report holds case %s in two development cohorts", c.CaseID)
		}
		out[c.CaseID] = c.Cohort
	}
	return out, nil
}

func resolveRevision(suite Suite, previous *Report) (*developmentRevision, error) {
	cohorts, err := previousCohorts(previous)
	if err != nil || len(cohorts) == 0 {
		return nil, err
	}
	if previous.FixtureHashes.Development == suite.Hashes.Development {
		for _, c := range suite.Development {
			if cohort, ok := cohorts[c.ID]; ok && cohort != developmentCohort(c) {
				return nil, fmt.Errorf("case %s changed development cohort under an unchanged development fixture", c.ID)
			}
		}
		return nil, nil
	}
	fixture, err := loadPreviousDevelopment(suite, previous.FixtureHashes.Development)
	if err != nil {
		return nil, err
	}
	expected := map[string]int{}
	for _, c := range fixture {
		expected[c.ID] = len(c.ExpectedCitations)
	}
	for _, c := range previous.Cases {
		if cohorts[c.CaseID] == c.Cohort {
			if n, ok := expected[c.CaseID]; ok && (c.Expected != n || len(c.Ranks) != n) {
				return nil, fmt.Errorf("previous report case %s differs from its development fixture", c.CaseID)
			}
		}
	}
	revision, err := compareFixtures(fixture, suite.Development, cohorts)
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

func sameFixtureContent(a, b Case) (bool, error) {
	type content struct {
		Question          string        `json:"question"`
		ExpectedCitations []Expectation `json:"expected_citations,omitempty"`
		ExpectAbstain     bool          `json:"expect_abstain,omitempty"`
	}
	x, err := json.Marshal(content{a.Question, a.ExpectedCitations, a.ExpectAbstain})
	if err != nil {
		return false, fmt.Errorf("encode case %s: %w", a.ID, err)
	}
	y, err := json.Marshal(content{b.Question, b.ExpectedCitations, b.ExpectAbstain})
	if err != nil {
		return false, fmt.Errorf("encode case %s: %w", b.ID, err)
	}
	return bytes.Equal(x, y), nil
}

func compareFixtures(previous, current []Case, cohorts map[string]string) (developmentRevision, error) {
	before := make(map[string]Case, len(previous))
	for _, c := range previous {
		if _, ok := cohorts[c.ID]; !ok {
			return developmentRevision{}, fmt.Errorf("previous report has no case records for development case %s", c.ID)
		}
		before[c.ID] = c
	}
	if len(cohorts) != len(before) {
		return developmentRevision{}, fmt.Errorf("previous report holds development cases outside its development fixture")
	}
	after := make(map[string]bool, len(current))
	for _, c := range current {
		after[c.ID] = true
	}
	for _, c := range previous {
		if !after[c.ID] {
			return developmentRevision{}, fmt.Errorf("development fixture revision removes case %s", c.ID)
		}
	}
	out := developmentRevision{comparable: []string{}, previousOrder: []string{}, revised: []RevisedCase{}}
	comparable := map[string]bool{}
	for _, c := range current {
		cohort := developmentCohort(c)
		old, existed := before[c.ID]
		reason := ""
		switch {
		case !existed:
			reason = "added"
		case cohorts[c.ID] != cohort:
			reason = "cohort"
		default:
			same, err := sameFixtureContent(old, c)
			if err != nil {
				return developmentRevision{}, err
			}
			if !same {
				reason = "changed"
			}
		}
		if reason == "" {
			if cohort == "development" {
				comparable[c.ID] = true
				out.comparable = append(out.comparable, c.ID)
			}
			continue
		}
		if cohort == "development" || cohorts[c.ID] == "development" {
			out.revised = append(out.revised, RevisedCase{c.ID, reason})
		}
	}
	for _, c := range previous {
		if comparable[c.ID] {
			out.previousOrder = append(out.previousOrder, c.ID)
		}
	}
	sort.Slice(out.revised, func(i, j int) bool { return out.revised[i].CaseID < out.revised[j].CaseID })
	return out, nil
}
