package discover

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/pinned"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/retrieval"
)

type secondaryDefinition struct {
	Source  string `json:"source"`
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Keyword string `json:"keyword,omitempty"`
	Number  string `json:"number,omitempty"`
	Date    string `json:"date,omitempty"`
	Cap     int    `json:"cap"`
}
type secondaryReport struct {
	Notices             []pinned.Notice          `json:"notices,omitempty"`
	Version             int                      `json:"version"`
	Definition          secondaryDefinition      `json:"definition"`
	QueryIdentity       string                   `json:"query_identity"`
	WireIdentity        string                   `json:"wire_identity"`
	SourceListHash      string                   `json:"source_list_hash"`
	IssuerPolicyVersion int                      `json:"issuer_policy_version"`
	Passes              []lookup.Result          `json:"passes"`
	Stable              bool                     `json:"stable"`
	Candidates          []secondaryCandidate     `json:"candidates"`
	Filtered            []secondaryFiltered      `json:"filtered"`
	FilterOutcomes      []secondaryFilterOutcome `json:"filter_outcomes"`
	Gaps                []stage.Gap              `json:"gaps"`
}
type secondaryFilterOutcome struct {
	Pass  int         `json:"pass"`
	Item  lookup.Item `json:"item"`
	State string      `json:"state"`
}
type secondaryFiltered struct {
	ID      string        `json:"id"`
	State   string        `json:"state"`
	Listing []lookup.Item `json:"listing"`
}
type secondaryCandidate struct {
	ID          string                `json:"id"`
	State       string                `json:"state"`
	Listing     []lookup.Item         `json:"listing"`
	DetailURL   string                `json:"detail_url"`
	DetailPath  string                `json:"detail_path"`
	DetailHash  string                `json:"detail_hash"`
	Fields      any                   `json:"fields"`
	Issuer      string                `json:"issuer"`
	IssuerForm  string                `json:"issuer_form"`
	MatchedForm string                `json:"matched_form"`
	Number      string                `json:"number"`
	Date        string                `json:"date"`
	ProbeState  string                `json:"probe_state"`
	ProbePasses []lookup.Result       `json:"probe_passes"`
	Comparisons []secondaryComparison `json:"comparisons"`
	PrimaryID   string                `json:"primary_id,omitempty"`
}

func secondaryItem(key string, rules source.ChinhphuSources) (secondaryDefinition, error) {
	d := secondaryDefinition{Source: "chinhphu", Key: key, Cap: 500}
	for _, word := range rules.Keywords {
		if key == "chinhphu:keyword:"+word {
			d.Kind = "keyword"
			d.Keyword = word
			return d, nil
		}
	}
	for _, name := range rules.NamedNumbers {
		if key == "chinhphu:named:"+name {
			n, e := source.ParseNamedNumber(name)
			if e != nil {
				return d, fmt.Errorf("parse named query: %w", e)
			}
			d.Kind = "named"
			d.Number = n.Number
			d.Date = n.Date
			return d, nil
		}
	}
	return d, fmt.Errorf("unknown secondary query %q", key)
}
func secondaryIdentity(d secondaryDefinition) string {
	b, _ := json.Marshal(d)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}
func (s *Stage) processSecondary(ctx context.Context, env stage.Env, r stage.Run, it stage.Item, rules source.ChinhphuSources) (stage.Output, error) {
	out := stage.Output{Sources: []string{"chinhphu", "vbpl"}}
	if s.chinhphu == nil || s.client == nil {
		return out, fmt.Errorf("secondary and VBPL clients are required")
	}
	d, err := secondaryItem(it.Key, rules)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dir string) error { return s.secondary(ctx, dir, r, d, rules) })
	if err != nil {
		return out, fmt.Errorf("publish secondary discovery: %w", err)
	}
	out.Artifact = &art
	out.State = "done"
	return out, nil
}
func (s *Stage) secondary(ctx context.Context, dir string, r stage.Run, d secondaryDefinition, rules source.ChinhphuSources) error {
	report := secondaryReport{Version: 1, Definition: d, QueryIdentity: secondaryIdentity(d), SourceListHash: r.Snapshot.SourceListHash, IssuerPolicyVersion: rules.IssuerPolicyVersion, Passes: []lookup.Result{}, Candidates: []secondaryCandidate{}, Gaps: []stage.Gap{}}
	query := secondarySearchText(d)
	union := map[string][]lookup.Item{}
	var last map[string][]lookup.Item
	var previous []string
	searchFailed := false
	for pass := 1; pass <= 3; pass++ {
		result, bodies, err := s.chinhphu.Search(ctx, query)
		definition, _ := json.Marshal(map[string]any{"source": "chinhphu", "type": "keyword", "keyword": query, "issuer_filter": "none", "cap": 500})
		wireHash := sha256.Sum256(definition)
		result.Evidence.Query = definition
		result.Evidence.QueryIdentity = fmt.Sprintf("%x", wireHash)
		if errors.Is(err, sourcegate.ErrSourcePaused) {
			return stage.Retryable(fmt.Errorf("secondary search: %w", err))
		}
		for i, body := range bodies {
			ref, e := writeSecondaryPage(dir, "search", pass, i+1, body)
			if e != nil {
				return e
			}
			result.Evidence.PagesFetched[i].Path = ref.Path
		}
		report.Passes = append(report.Passes, result)
		report.WireIdentity = result.Evidence.QueryIdentity
		current := map[string][]lookup.Item{}
		for _, item := range result.Items {
			if d.Kind == "named" {
				state := "eligible_for_detail"
				if item.RawNumber == nil || !knownListingNumber(*item.RawNumber) {
					state = "uncertain"
				} else if retrieval.NormalizeNumber(*item.RawNumber) != retrieval.NormalizeNumber(d.Number) {
					state = "unequal_number"
				}
				report.FilterOutcomes = append(report.FilterOutcomes, secondaryFilterOutcome{Pass: pass, Item: item, State: state})
			}
			current[item.SourceID] = append(current[item.SourceID], item)
			union[item.SourceID] = append(union[item.SourceID], item)
		}
		last = current
		currentIDs := ids(result.Items)
		if err != nil {
			report.Gaps = append(report.Gaps, secondaryFailure("chinhphu", err))
			searchFailed = true
			break
		}
		if pass > 1 && result.Evidence.Completeness == lookup.Complete && report.Passes[pass-2].Evidence.Completeness == lookup.Complete && sameIDs(previous, currentIDs) {
			report.Stable = true
			break
		}
		previous = currentIDs
	}
	if !report.Stable && !searchFailed {
		report.Gaps = append(report.Gaps, stage.Gap{Kind: "listing_unstable", Detail: d.Key})
	} else if report.Stable {
		union = last
	}
	keys := make([]string, 0, len(union))
	for id, rows := range union {
		keep := d.Kind != "named"
		for _, row := range rows {
			if row.RawNumber == nil || !knownListingNumber(*row.RawNumber) || retrieval.NormalizeNumber(*row.RawNumber) == retrieval.NormalizeNumber(d.Number) {
				keep = true
			}
		}
		if keep {
			keys = append(keys, id)
		} else {
			report.Filtered = append(report.Filtered, secondaryFiltered{ID: id, State: "unequal_number", Listing: rows})
		}
	}
	sort.Strings(keys)
	sort.Slice(report.Filtered, func(i, j int) bool { return report.Filtered[i].ID < report.Filtered[j].ID })
	for _, id := range keys {
		report.Candidates = append(report.Candidates, secondaryCandidate{ID: id, State: "not_attempted", Listing: union[id], ProbeState: "not_attempted"})
	}
	if len(keys) <= s.candidateCeiling() {
		for i := range report.Candidates {
			c := &report.Candidates[i]
			id, e := strconv.Atoi(c.ID)
			if e != nil || id <= 0 {
				c.State = "invalid"
				report.Gaps = append(report.Gaps, stage.Gap{Kind: "invalid_source_data", Detail: c.ID})
				continue
			}
			if e = s.secondaryDetail(ctx, dir, c, id, d, rules, &report); e != nil {
				return e
			}
		}
	}
	if d.Kind == "named" && !report.Stable {
		for i := range report.Candidates {
			c := &report.Candidates[i]
			if c.State == "admit_secondary" || c.State == "seed_primary" {
				c.State = "incomplete"
			}
		}
	}
	if d.Kind == "named" && report.Stable && len(keys) <= s.candidateCeiling() {
		eligible, unresolved := 0, 0
		for _, c := range report.Candidates {
			switch c.State {
			case "admit_secondary", "seed_primary":
				eligible++
			case "incomplete", "invalid", "ambiguous":
				unresolved++
			}
		}
		if unresolved > 0 || eligible > 1 {
			report.Gaps = append(report.Gaps, stage.Gap{Kind: "named_document_ambiguous", Detail: d.Key})
			for i := range report.Candidates {
				if report.Candidates[i].State == "admit_secondary" || report.Candidates[i].State == "seed_primary" {
					report.Candidates[i].State = "named_ambiguous"
				}
			}
		} else if eligible == 0 {
			report.Gaps = append(report.Gaps, stage.Gap{Kind: "named_document_missing", Detail: d.Key})
		}
	}
	for i := range report.Candidates {
		c := report.Candidates[i]
		if err := writeJSON(dir, "candidate-"+c.ID+".json", secondaryCandidateEvidence(report, c)); err != nil {
			return err
		}
	}
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return fmt.Errorf("parse pin notices: %w", err)
	}
	live := map[string]bool{}
	for _, candidate := range report.Candidates {
		if (candidate.ProbeState != "complete" && candidate.ProbeState != "ambiguous") || len(candidate.ProbePasses) == 0 {
			continue
		}
		for _, notice := range livePins(ids(candidate.ProbePasses[len(candidate.ProbePasses)-1].Items), list.VBPL.Pinned, true) {
			live[notice.Key] = true
		}
	}
	for key := range live {
		report.Notices = append(report.Notices, pinned.Notice{Kind: "source_pin_live", Key: key})
	}
	sort.Slice(report.Notices, func(i, j int) bool { return report.Notices[i].Key < report.Notices[j].Key })
	return writeJSON(dir, "secondary.json", report)
}

type secondaryEvidence struct {
	SearchPath          string             `json:"search_path"`
	ItemIdentity        string             `json:"item_identity"`
	WireIdentity        string             `json:"wire_identity"`
	SourceListHash      string             `json:"source_list_hash"`
	IssuerPolicyVersion int                `json:"issuer_policy_version"`
	Candidate           secondaryCandidate `json:"candidate"`
}

func secondaryCandidateEvidence(report secondaryReport, c secondaryCandidate) secondaryEvidence {
	return secondaryEvidence{SearchPath: "secondary.json", ItemIdentity: report.QueryIdentity, WireIdentity: report.WireIdentity, SourceListHash: report.SourceListHash, IssuerPolicyVersion: report.IssuerPolicyVersion, Candidate: c}
}
