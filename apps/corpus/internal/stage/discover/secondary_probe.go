package discover

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"danny.vn/trachuan/internal/retrieval"
)

type secondaryComparison struct {
	ID    string      `json:"id"`
	State string      `json:"state"`
	Item  lookup.Item `json:"item"`
}

func secondaryFailure(name string, err error) stage.Gap {
	g := stage.Gap{Kind: "source_unavailable", Detail: err.Error(), Source: name, Origin: stage.OriginUnknown, Cause: stage.CauseUnknown}
	var cp *chinhphu.Failure
	var vb *vbpl.Failure
	if errors.As(err, &cp) {
		g.Origin = stage.Origin(cp.Origin)
		g.Cause = stage.Cause(cp.Cause)
		g.Endpoint = cp.Endpoint
		if cp.HTTPStatus != 0 {
			s := int32(cp.HTTPStatus)
			g.HTTPStatus = &s
		}
	}
	if errors.As(err, &vb) {
		g.Origin = stage.Origin(vb.Origin)
		g.Cause = stage.Cause(vb.Cause)
		g.Endpoint = vb.Endpoint
		if vb.HTTPStatus != 0 {
			s := int32(vb.HTTPStatus)
			g.HTTPStatus = &s
		}
	}
	if name == "vbpl" && g.Origin == stage.OriginUnknown && errors.Is(err, io.EOF) {
		g.Origin = stage.OriginTransport
		g.Cause = stage.CauseConnectionError
	}
	switch {
	case errors.Is(err, chinhphu.ErrNotFound), errors.Is(err, vbpl.ErrNotFound):
		g.Kind = "not_found"
	case errors.Is(err, chinhphu.ErrTooLarge), errors.Is(err, vbpl.ErrTooLarge):
		g.Kind = "response_too_large"
	case g.Origin == stage.OriginSourceData:
		g.Kind = "invalid_source_data"
	}
	return g
}
func writeSecondaryPage(dir, prefix string, pass, page int, body []byte) (pageReference, error) {
	name := fmt.Sprintf("%s/p%d-%d", prefix, pass, page)
	if err := os.MkdirAll(filepath.Join(dir, prefix), 0700); err != nil {
		return pageReference{}, fmt.Errorf("create evidence directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
		return pageReference{}, fmt.Errorf("write evidence: %w", err)
	}
	h := sha256.Sum256(body)
	return pageReference{Pass: pass, Page: page, Path: name, SHA256: fmt.Sprintf("%x", h)}, nil
}
func fieldValue(fields []chinhphu.FieldOccurrence, name string, normalize func(string) string) (string, bool) {
	raw, normalized := "", ""
	found := false
	for _, f := range fields {
		if strings.TrimSpace(f.Name) != name {
			continue
		}
		if f.Ambiguous {
			return "", false
		}
		v := normalize(f.Value)
		if v == "" || found && v != normalized {
			return "", false
		}
		if !found {
			raw = strings.TrimSpace(f.Value)
		}
		normalized = v
		found = true
	}
	return raw, found
}
func dateValue(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04:05", "02-01-2006", "02/01/2006"} {
		if d, e := time.Parse(layout, s); e == nil {
			return d.Format("2006-01-02")
		}
	}
	return ""
}
func (s *Stage) secondaryDetail(ctx context.Context, dir string, c *secondaryCandidate, id int, d secondaryDefinition, rules source.ChinhphuSources, report *secondaryReport) error {
	detail, raw, err := s.chinhphu.Detail(ctx, id)
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return stage.Retryable(fmt.Errorf("secondary detail: %w", err))
	}
	if len(raw) > 0 {
		ref, e := writeSecondaryPage(dir, "detail", id, 1, raw)
		if e != nil {
			return e
		}
		c.DetailPath = ref.Path
		c.DetailHash = ref.SHA256
	}
	if err != nil {
		c.State = "incomplete"
		report.Gaps = append(report.Gaps, secondaryFailure("chinhphu", err))
		return nil
	}
	c.DetailURL = detail.URL
	c.Fields = detail.Occurrences
	issuer, _ := fieldValue(detail.Occurrences, "Cơ quan ban hành", source.NormalizeIssuer)
	number, numberOK := fieldValue(detail.Occurrences, "Số ký hiệu", retrieval.NormalizeNumber)
	date, dateOK := fieldValue(detail.Occurrences, "Ngày ban hành", dateValue)
	c.IssuerForm = issuer
	c.Number = number
	if dateOK {
		c.Date = dateValue(date)
	}
	c.Issuer = classifyIssuer(detail.Occurrences, rules.CentralIssuerForms)
	if c.Issuer == "provincial" {
		c.State = "provincial"
		return nil
	}
	if c.Issuer != "central" {
		report.Gaps = append(report.Gaps, stage.Gap{Kind: "invalid_source_data", Detail: c.ID + ": unknown issuer"})
	}
	if d.Kind == "named" && numberOK && (retrieval.NormalizeNumber(number) != retrieval.NormalizeNumber(d.Number) || (d.Date != "" && c.Date != "" && c.Date != d.Date)) {
		c.State = "ineligible"
		return nil
	}
	if c.Issuer != "central" {
		c.State = "invalid"
		return nil
	}
	for _, form := range rules.CentralIssuerForms {
		if source.NormalizeIssuer(form) == source.NormalizeIssuer(issuer) {
			c.MatchedForm = form
			break
		}
	}
	if !numberOK {
		c.State = "invalid"
		report.Gaps = append(report.Gaps, stage.Gap{Kind: "invalid_source_data", Detail: c.ID + ": invalid number"})
		return nil
	}
	if d.Kind == "named" {
		if retrieval.NormalizeNumber(number) != retrieval.NormalizeNumber(d.Number) {
			c.State = "ineligible"
			return nil
		}
		if c.Date == "" {
			c.State = "invalid"
			return nil
		}
		if d.Date != "" && c.Date != d.Date {
			c.State = "ineligible"
			return nil
		}
	}
	return s.secondaryProbe(ctx, dir, c, report)
}
func (s *Stage) secondaryProbe(ctx context.Context, dir string, c *secondaryCandidate, report *secondaryReport) error {
	var previous []string
	var selected lookup.Result
	union := map[string]lookup.Item{}
	probeFailed := false
	for pass := 1; pass <= 3; pass++ {
		result, bodies, err := s.client.LookupNumber(ctx, secondaryWireNumber(c.Number))
		if errors.Is(err, sourcegate.ErrSourcePaused) {
			return stage.Retryable(fmt.Errorf("VBPL probe: %w", err))
		}
		for i, body := range bodies {
			ref, e := writeSecondaryPage(dir, "probe-"+c.ID, pass, i+1, body)
			if e != nil {
				return e
			}
			result.Evidence.PagesFetched[i].Path = ref.Path
		}
		c.ProbePasses = append(c.ProbePasses, result)
		selected = result
		for _, item := range result.Items {
			union[item.SourceID] = item
		}
		if err != nil {
			c.State = "incomplete"
			c.ProbeState = "incomplete"
			report.Gaps = append(report.Gaps, secondaryFailure("vbpl", err))
			probeFailed = true
			break
		}
		current := ids(result.Items)
		if pass > 1 && result.Evidence.Completeness == lookup.Complete && c.ProbePasses[pass-2].Evidence.Completeness == lookup.Complete && sameIDs(previous, current) {
			c.ProbeState = "complete"
			break
		}
		previous = current
	}
	if c.ProbeState != "complete" {
		c.State = "incomplete"
		c.ProbeState = "incomplete"
		if !probeFailed {
			report.Gaps = append(report.Gaps, stage.Gap{Kind: "listing_unstable", Detail: "VBPL probe " + c.ID})
		}
		selected.Items = nil
		keys := make([]string, 0, len(union))
		for id := range union {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			selected.Items = append(selected.Items, union[id])
		}
	}
	matches, unknown := 0, 0
	for _, item := range selected.Items {
		state := "different_number"
		if item.RawNumber == nil || retrieval.NormalizeNumber(*item.RawNumber) == "" {
			state = "ambiguous"
		} else if retrieval.NormalizeNumber(*item.RawNumber) == retrieval.NormalizeNumber(c.Number) {
			state = compareSecondary(c, item)
		}
		c.Comparisons = append(c.Comparisons, secondaryComparison{ID: item.SourceID, State: state, Item: item})
		if state == "match" {
			matches++
			c.PrimaryID = item.SourceID
		}
		if state == "ambiguous" {
			unknown++
		}
	}
	if c.ProbeState == "incomplete" {
		return nil
	}
	switch {
	case unknown > 0 || matches > 1:
		c.State = "ambiguous"
		c.ProbeState = "ambiguous"
		report.Gaps = append(report.Gaps, stage.Gap{Kind: "source_identity_ambiguous", Detail: c.ID})
	case matches == 1:
		c.State = "seed_primary"
	default:
		c.State = "admit_secondary"
	}
	return nil
}
func compareSecondary(c *secondaryCandidate, item lookup.Item) string {
	date := ""
	if item.IssueDate != nil {
		date = dateValue(*item.IssueDate)
	}
	if c.Date != "" && date != "" && c.Date != date {
		return "different_date"
	}
	if c.Date == "" || date == "" || item.Issuer == nil || source.NormalizeIssuer(*item.Issuer) == "" || source.NormalizeIssuer(*item.Issuer) != source.NormalizeIssuer(c.IssuerForm) {
		return "ambiguous"
	}
	return "match"
}
