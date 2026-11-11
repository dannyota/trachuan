package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/internal/retrieval"
)

// Options identifies the inputs and environment of an evaluation.
type Options struct {
	Mapping              ValidationReport
	EvaluationDate       retrieval.Date
	Modes                []retrieval.Mode
	PreviousReport       string
	BuildArtifactSHA256  string
	DumpSHA256           string
	DatabaseInstanceKind string
	Recipes              map[string]string
	Software             map[string]string
	Calibration          any
}

// Metric records a measured ratio with its counts.
type Metric struct {
	Mode        retrieval.Mode    `json:"mode"`
	Arms        retrieval.ArmMode `json:"arms"`
	Limit       int               `json:"limit"`
	Name        string            `json:"name"`
	Numerator   float64           `json:"numerator"`
	Denominator int               `json:"denominator"`
	Value       *float64          `json:"value"`
	Confusion   *Confusion        `json:"confusion,omitempty"`
}

// Cohort summarizes the cases and metrics for one scoring policy.
type Cohort struct {
	Floors            []Floor         `json:"floors,omitempty"`
	Targets           []Target        `json:"targets,omitempty"`
	NonRegression     []NonRegression `json:"non_regression,omitempty"`
	CaseCount         int             `json:"case_count"`
	PositiveCount     int             `json:"positive_count"`
	NegativeCount     int             `json:"negative_count"`
	RejectionCount    int             `json:"rejection_count"`
	ExpectedCitations int             `json:"expected_citations"`
	Metrics           []Metric        `json:"metrics"`
}

// Floor records a product-cohort quality gate.
type Floor struct {
	Metric
	Floor  float64 `json:"floor"`
	Status string  `json:"status"`
}

// Target records a goal that does not affect quality.
type Target struct {
	Metric
	Target float64 `json:"target"`
	Status string  `json:"status"`
}

// NonRegression compares a metric with its previous value.
type NonRegression struct {
	Mode          retrieval.Mode    `json:"mode"`
	Arms          retrieval.ArmMode `json:"arms"`
	Limit         int               `json:"limit"`
	Name          string            `json:"name"`
	Value         *float64          `json:"value"`
	PreviousValue *float64          `json:"previous_value"`
	Status        string            `json:"status"`
}

// SizeSummary records nearest-rank evidence sizes.
type SizeSummary struct {
	Cohort string            `json:"cohort"`
	Mode   retrieval.Mode    `json:"mode"`
	Arms   retrieval.ArmMode `json:"arms"`
	Limit  int               `json:"limit"`
	N      int               `json:"n"`
	P50    *float64          `json:"p50"`
	P95    *float64          `json:"p95"`
}

// CaseResult contains the non-sensitive result of one response.
type CaseResult struct {
	MappingIssues []MappingIssue       `json:"mapping_issues,omitempty"`
	CaseID        string               `json:"case_id"`
	Cohort        string               `json:"cohort"`
	Mode          retrieval.Mode       `json:"mode"`
	Arms          retrieval.ArmMode    `json:"arms"`
	Limit         int                  `json:"limit"`
	Expected      int                  `json:"expected"`
	Ranks         []int                `json:"ranks"`
	DirectRanks   []int                `json:"direct_ranks,omitempty"`
	Returned      []retrieval.Identity `json:"returned"`
	Mapped        [][]Alternative      `json:"mapped_identities"`
	Matched       int                  `json:"matched"`
	Losses        []CitationLoss       `json:"losses"`
	Abstain       bool                 `json:"abstain"`
	Reason        string               `json:"reason,omitempty"`
	Gaps          []retrieval.Gap      `json:"gaps"`
	Characters    int                  `json:"characters"`
}

// Rejection records a product input gate check.
type Rejection struct {
	CaseID string `json:"case_id"`
	Reason string `json:"reason"`
	Passed bool   `json:"passed"`
}

// PresentedSummary records matching and serialized response sizes.
type PresentedSummary struct {
	Cohort   string           `json:"cohort"`
	Mode     retrieval.Mode   `json:"mode"`
	Detail   retrieval.Detail `json:"detail"`
	Limit    int              `json:"limit"`
	Cases    int              `json:"cases"`
	Expected int              `json:"expected"`
	Found    int              `json:"found"`
	SizeP50  *float64         `json:"size_p50"`
	SizeP95  *float64         `json:"size_p95"`
}

// CompactSizeGate compares compact and full response sizes.
type CompactSizeGate struct {
	FullP95At8     *float64 `json:"full_p95_at_8"`
	CompactP95At20 *float64 `json:"compact_p95_at_20"`
	Pass           bool     `json:"pass"`
}

// Report is a completed diagnostic evaluation.
type Report struct {
	Presented                     []PresentedSummary        `json:"presented"`
	CompactSizeGate               CompactSizeGate           `json:"compact_size_gate"`
	DocumentsByClass              map[string]int            `json:"documents_by_class"`
	DocumentsBySource             map[string]int            `json:"documents_by_source"`
	DocumentsByTextSource         map[string]int            `json:"documents_by_text_source"`
	DocumentsByTextSourceAndClass map[string]map[string]int `json:"documents_by_text_source_and_class"`
	DocumentsTextIncomplete       int                       `json:"documents_text_incomplete"`
	HitsByClass                   map[string]int            `json:"hits_by_class"`
	HitsByStatusBasis             map[string]int            `json:"hits_by_status_basis"`
	HitsByConflictKind            map[string]int            `json:"hits_by_conflict_kind"`
	ValidityAdjudicated           AdjudicatedValidity       `json:"validity_adjudicated"`
	Version                       int                       `json:"version"`
	Scorer                        string                    `json:"scorer"`
	Status                        string                    `json:"status"`
	QualityPass                   bool                      `json:"quality_pass"`
	ReleaseEligible               bool                      `json:"release_eligible"`
	Country                       string                    `json:"country"`
	Rel                           string                    `json:"rel"`
	BuildArtifactSHA256           string                    `json:"build_artifact_sha256"`
	DumpSHA256                    string                    `json:"dump_sha256"`
	FixtureHashes                 FixtureHashes             `json:"fixture_hashes"`
	IdentityMapSHA256             string                    `json:"identity_map_sha256"`
	Recipes                       map[string]string         `json:"recipes"`
	EvaluationDate                retrieval.Date            `json:"evaluation_date"`
	RequestAsOf                   *retrieval.Date           `json:"request_as_of"`
	DatabaseInstanceKind          string                    `json:"database_instance_kind"`
	Software                      map[string]string         `json:"software"`
	Calibration                   any                       `json:"calibration"`
	Cohorts                       map[string]Cohort         `json:"cohorts"`
	Targets                       []Target                  `json:"targets"`
	NonRegression                 []NonRegression           `json:"non_regression"`
	Floors                        []Floor                   `json:"floors"`
	Rejections                    []Rejection               `json:"rejections"`
	Coverage                      []Coverage                `json:"coverage"`
	ANN                           []ANNMeasure              `json:"-"`
	ANNResults                    map[string]ANNLoss        `json:"ann_loss"`
	TimingsMS                     map[string]any            `json:"timings_ms"`
	EvidenceCharacters            []SizeSummary             `json:"evidence_characters"`
	CacheManifest                 map[string]any            `json:"cache_manifest"`
	PreviousDiff                  any                       `json:"previous_diff"`
	NotRun                        map[string]string         `json:"not_run"`
	Limitations                   []string                  `json:"limitations"`
	Cases                         []CaseResult              `json:"cases"`
}

// ANNMeasure reports candidate overlap before and after fallback.
type ANNMeasure struct {
	CaseID           string `json:"case_id"`
	Cohort           string `json:"cohort"`
	ExactCount       int    `json:"exact_count"`
	RawOverlap       int    `json:"raw_overlap"`
	EffectiveOverlap int    `json:"effective_overlap"`
	Fallback         bool   `json:"fallback"`
}

// WriteFiles writes the report and per-response rows.
func (r Report) WriteFiles(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	if err := writeJSON(filepath.Join(dir, "report.json"), r); err != nil {
		return err
	}
	if err := writeJSONLines(filepath.Join(dir, "cases.jsonl"), r.Cases); err != nil {
		return err
	}
	return checkArtifactSize(dir)
}
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	b = append(b, '\n')
	if err = os.WriteFile(path, b, 0600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
func writeJSONLines[T any](path string, rows []T) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	enc := json.NewEncoder(f)
	for _, row := range rows {
		if err = enc.Encode(row); err != nil {
			_ = f.Close()
			return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
		}
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	return nil
}

// CaseDifference lists changed citation matches and abstention decisions.
type CaseDifference struct {
	CaseID          string            `json:"case_id"`
	Cohort          string            `json:"cohort"`
	Mode            retrieval.Mode    `json:"mode"`
	Arms            retrieval.ArmMode `json:"arms"`
	Limit           int               `json:"limit"`
	Gained          []int             `json:"gained"`
	Lost            []int             `json:"lost"`
	PreviousAbstain bool              `json:"previous_abstain"`
	Abstain         bool              `json:"abstain"`
}

func compareCases(previous, current []CaseResult) []CaseDifference {
	type key struct {
		id, cohort string
		mode       retrieval.Mode
		arms       retrieval.ArmMode
		limit      int
	}
	old := map[key]CaseResult{}
	for _, c := range previous {
		old[key{c.CaseID, c.Cohort, c.Mode, c.Arms, c.Limit}] = c
	}
	changes := []CaseDifference{}
	for _, c := range current {
		p, ok := old[key{c.CaseID, c.Cohort, c.Mode, c.Arms, c.Limit}]
		if !ok {
			continue
		}
		d := CaseDifference{CaseID: c.CaseID, Cohort: c.Cohort, Mode: c.Mode, Arms: c.Arms, Limit: c.Limit, PreviousAbstain: p.Abstain, Abstain: c.Abstain}
		for i := 0; i < max(len(p.Ranks), len(c.Ranks)); i++ {
			before := i < len(p.Ranks) && p.Ranks[i] > 0
			after := i < len(c.Ranks) && c.Ranks[i] > 0
			if after && !before {
				d.Gained = append(d.Gained, i)
			}
			if before && !after {
				d.Lost = append(d.Lost, i)
			}
		}
		if len(d.Gained) > 0 || len(d.Lost) > 0 || d.PreviousAbstain != d.Abstain {
			changes = append(changes, d)
		}
	}
	return changes
}

// ANNLoss summarizes candidate overlap and hybrid score differences.
type ANNLoss struct {
	Cases             int              `json:"cases"`
	ExactCandidates   int              `json:"exact_candidates"`
	RawMatches        int              `json:"raw_matches"`
	EffectiveMatches  int              `json:"effective_matches"`
	RawRecall         *float64         `json:"raw_recall"`
	EffectiveRecall   *float64         `json:"effective_recall"`
	Fallbacks         int              `json:"fallbacks"`
	FallbackFrequency *float64         `json:"fallback_frequency"`
	Recall8PP         *float64         `json:"recall_8_pp"`
	MRR8PP            *float64         `json:"mrr_8_pp"`
	Recall20PP        *float64         `json:"recall_20_pp"`
	Differences       []CaseDifference `json:"differences"`
}

func summarizeANN(r Report, cohort string) ANNLoss {
	var out ANNLoss
	for _, v := range r.ANN {
		out.Cases++
		out.ExactCandidates += v.ExactCount
		out.RawMatches += v.RawOverlap
		out.EffectiveMatches += v.EffectiveOverlap
		if v.Fallback {
			out.Fallbacks++
		}
	}
	out.RawRecall = ratio(out.RawMatches, out.ExactCandidates)
	out.EffectiveRecall = ratio(out.EffectiveMatches, out.ExactCandidates)
	out.FallbackFrequency = ratio(out.Fallbacks, out.Cases)
	metrics := r.Cohorts[cohort].Metrics
	delta := func(name string, limit int) *float64 {
		var exact, ann *float64
		for _, m := range metrics {
			if m.Name == name && m.Limit == limit && m.Arms == "hybrid" {
				if m.Mode == "exact" {
					exact = m.Value
				}
				if m.Mode == "ann" {
					ann = m.Value
				}
			}
		}
		if exact == nil || ann == nil {
			return nil
		}
		v := 100 * (*exact - *ann)
		return &v
	}
	out.Recall8PP = delta("recall", 8)
	out.MRR8PP = delta("mrr", 8)
	out.Recall20PP = delta("recall", 20)
	var exact, ann []CaseResult
	for _, c := range r.Cases {
		if c.Cohort != cohort || c.Arms != "hybrid" {
			continue
		}
		switch c.Mode {
		case "exact":
			c.Mode = "ann"
			exact = append(exact, c)
		case "ann":
			ann = append(ann, c)
		}
	}
	out.Differences = compareCases(exact, ann)
	return out
}

func addLegacyDirectMetrics(r *Report) {
	cohort := r.Cohorts["legacy"]
	for _, mode := range []retrieval.Mode{"exact", "ann"} {
		for _, arm := range []retrieval.ArmMode{"hybrid", "dense", "lexical"} {
			for _, limit := range []int{8, 20} {
				total, matched, positive := 0, 0, 0
				macro, mrr := 0.0, 0.0
				for _, c := range r.Cases {
					if c.Cohort != "legacy" || c.Mode != mode || c.Arms != arm || c.Limit != limit || c.Expected == 0 {
						continue
					}
					positive++
					n, best := 0, 0
					for _, rank := range c.DirectRanks {
						if rank > 0 && rank <= limit {
							n++
						}
						if rank > 0 && rank <= 8 && (best == 0 || rank < best) {
							best = rank
						}
					}
					total += c.Expected
					matched += n
					macro += float64(n) / float64(c.Expected)
					if best > 0 {
						mrr += 1 / float64(best)
					}
				}
				if positive == 0 {
					continue
				}
				add := func(name string, num float64, den int) {
					v := num / float64(den)
					cohort.Metrics = append(cohort.Metrics, Metric{Mode: mode, Arms: arm, Limit: limit, Name: name, Numerator: num, Denominator: den, Value: &v})
				}
				add("direct_recall", float64(matched), total)
				add("direct_macro_recall", macro, positive)
				if limit == 8 {
					add("direct_mrr", mrr, positive)
				}
			}
		}
	}
	r.Cohorts["legacy"] = cohort
}
