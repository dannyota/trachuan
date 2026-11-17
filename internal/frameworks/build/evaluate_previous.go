package build

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type previousReference struct {
	Rel          string `json:"rel"`
	ReportSHA256 string `json:"report_sha256"`
}

type previousComparison struct {
	Status  string                  `json:"status"`
	Reason  string                  `json:"reason"`
	Pass    bool                    `json:"pass"`
	Cohorts map[string]pairedCohort `json:"cohorts"`
	Changes cohortChanges           `json:"cohort_changes"`
}

type pairedCohort struct {
	Caller string                `json:"caller"`
	Modes  map[string]pairedMode `json:"modes"`
}

type pairedMode struct {
	Cases          []string `json:"case_ids"`
	PreviousRecall Metric   `json:"previous_recall_at_8"`
	CurrentRecall  Metric   `json:"current_recall_at_8"`
	PreviousMRR    Metric   `json:"previous_mrr_at_8"`
	CurrentMRR     Metric   `json:"current_mrr_at_8"`
	LostHits       []string `json:"lost_hits"`
	Pass           bool     `json:"pass"`
}

type cohortChanges struct {
	Added            []string `json:"added"`
	Removed          []string `json:"removed"`
	Rejected         []string `json:"rejected"`
	Retargeted       []string `json:"retargeted"`
	NoLongerAccepted []string `json:"no_longer_accepted"`
	NewlyInRelease   []string `json:"newly_in_release"`
	NewVersions      []string `json:"newly_in_release_versions"`
}

type previousReport struct {
	Version     int               `json:"version"`
	Rel         string            `json:"rel"`
	QualityPass bool              `json:"quality_pass"`
	Cases       []CaseResult      `json:"cases"`
	CaseCounts  []GoldenCaseCount `json:"case_counts"`
}

func readPreviousReport(base, rel string) (*previousReference, previousReport, error) {
	raw, err := optionalArtifact(filepath.Join(base, "snapshot/trachuan/previous-report.json"))
	if err != nil || raw == nil {
		return nil, previousReport{}, err
	}
	var previous previousReport
	if err = json.Unmarshal(raw, &previous); err != nil {
		return nil, previousReport{}, hide("decode previous report", err)
	}
	if previous.Version != 1 || !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(previous.Rel) || previous.Rel == rel || !previous.QualityPass || previous.Cases == nil {
		return nil, previousReport{}, fmt.Errorf("invalid previous report")
	}
	seen := map[string]bool{}
	for _, c := range previous.Cases {
		if strings.TrimSpace(c.ID) == "" || strings.ContainsAny(c.ID, "\r\n") || seen[c.ID] {
			return nil, previousReport{}, fmt.Errorf("invalid previous case identity")
		}
		seen[c.ID] = true
		switch c.Adjudication {
		case "accepted", "rejected", "unverified", "out_of_release":
		default:
			return nil, previousReport{}, fmt.Errorf("invalid previous case adjudication")
		}
		if c.Adjudication == "accepted" && !c.Negative && c.Target == "" {
			return nil, previousReport{}, fmt.Errorf("missing previous target")
		}
	}
	return &previousReference{Rel: previous.Rel, ReportSHA256: digest(raw)}, previous, nil
}

func comparePrevious(reference *previousReference, before previousReport, after []CaseResult, afterCounts []GoldenCaseCount) previousComparison {
	result := previousComparison{Status: "not_applicable", Reason: "The previous signed report input is absent.", Pass: true, Cohorts: map[string]pairedCohort{}}
	if reference == nil {
		return result
	}
	result.Status, result.Reason = "evaluated", ""
	result.Changes = compareCohortChanges(before.Cases, after)
	result.Changes.NewVersions = newlyInReleaseVersions(before.CaseCounts, afterCounts)
	for _, cohort := range []string{"public", "licensed"} {
		c := pairedCohort{Caller: callerFor(cohort), Modes: map[string]pairedMode{}}
		for _, mode := range []string{"open", "filtered"} {
			m := comparePairedMode(before.Cases, after, cohort, mode)
			c.Modes[mode] = m
			result.Pass = result.Pass && m.Pass
		}
		result.Cohorts[cohort] = c
	}
	return result
}

func comparePairedMode(before, after []CaseResult, cohort, mode string) pairedMode {
	const reciprocalScale = 840
	old := map[string]CaseResult{}
	for _, c := range before {
		old[c.ID] = c
	}
	current := slices.Clone(after)
	slices.SortFunc(current, func(a, b CaseResult) int { return strings.Compare(a.ID, b.ID) })
	result := pairedMode{Cases: []string{}, LostHits: []string{}}
	previousHits, currentHits, previousUnits, currentUnits := 0, 0, 0, 0
	previousError, currentError := false, false
	for _, c := range current {
		previous, ok := old[c.ID]
		if !ok || c.Negative || previous.Negative || c.Target != previous.Target || !cohortMember(c, cohort) || !cohortMember(previous, cohort) {
			continue
		}
		result.Cases = append(result.Cases, c.ID)
		p, pValid := pairedOutcome(previous, cohort, mode)
		n, nValid := pairedOutcome(c, cohort, mode)
		previousError, currentError = previousError || !pValid, currentError || !nValid
		if pValid && p.Rank > 0 {
			previousHits++
			previousUnits += reciprocalScale / p.Rank
		}
		if nValid && n.Rank > 0 {
			currentHits++
			currentUnits += reciprocalScale / n.Rank
		}
		if pValid && p.Rank > 0 && (!nValid || n.Rank == 0) {
			result.LostHits = append(result.LostHits, c.ID)
		}
	}
	d := len(result.Cases)
	result.PreviousRecall = metric(float64(previousHits), d, previousError)
	result.CurrentRecall = metric(float64(currentHits), d, currentError)
	result.PreviousMRR = metric(float64(previousUnits)/reciprocalScale, d, previousError)
	result.CurrentMRR = metric(float64(currentUnits)/reciprocalScale, d, currentError)
	result.Pass = d > 0 && !previousError && !currentError && len(result.LostHits) == 0 && currentHits >= previousHits && currentUnits >= previousUnits
	return result
}

func pairedOutcome(c CaseResult, cohort, mode string) (CaseOutcome, bool) {
	outcome, ok := c.Cohorts[cohort][mode]
	return outcome, ok && !outcome.Error && outcome.Rank >= 0 && outcome.Rank <= 8 && outcome.Miss == (outcome.Rank == 0)
}

func compareCohortChanges(before, after []CaseResult) cohortChanges {
	result := cohortChanges{Added: []string{}, Removed: []string{}, Rejected: []string{}, Retargeted: []string{}, NoLongerAccepted: []string{}, NewlyInRelease: []string{}, NewVersions: []string{}}
	old := map[string]CaseResult{}
	current := map[string]CaseResult{}
	for _, c := range before {
		old[c.ID] = c
	}
	for _, c := range after {
		current[c.ID] = c
		p, ok := old[c.ID]
		if !ok {
			result.Added = append(result.Added, c.ID)
			continue
		}
		if c.Adjudication == "rejected" && p.Adjudication != "rejected" {
			result.Rejected = append(result.Rejected, c.ID)
		}
		if p.Adjudication == "accepted" && c.Adjudication != "accepted" {
			result.NoLongerAccepted = append(result.NoLongerAccepted, c.ID)
		}
		if p.Target != "" && c.Target != "" && p.Target != c.Target {
			result.Retargeted = append(result.Retargeted, c.ID)
		}
		if p.Adjudication == "out_of_release" && c.Adjudication != "out_of_release" && c.Target != "" {
			result.NewlyInRelease = append(result.NewlyInRelease, c.ID)
		}
	}
	for _, c := range before {
		if _, ok := current[c.ID]; !ok {
			result.Removed = append(result.Removed, c.ID)
		}
	}
	for _, ids := range [][]string{result.Added, result.Removed, result.Rejected, result.Retargeted, result.NoLongerAccepted, result.NewlyInRelease, result.NewVersions} {
		slices.Sort(ids)
	}
	return result
}

func newlyInReleaseVersions(before, after []GoldenCaseCount) []string {
	members := func(counts []GoldenCaseCount) map[string]bool {
		versions := map[string]bool{}
		for _, count := range counts {
			if count.Code != "" && count.Version != "" && (count.Accepted > 0 || count.Rejected > 0 || count.Unverified > 0) {
				versions["fw:"+count.Code+":"+count.Version] = true
			}
		}
		return versions
	}
	previous := members(before)
	added := []string{}
	for version := range members(after) {
		if !previous[version] {
			added = append(added, version)
		}
	}
	slices.Sort(added)
	return added
}
