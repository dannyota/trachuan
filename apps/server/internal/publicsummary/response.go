package publicsummary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type outline struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Anchor  string `json:"anchor"`
	Heading string `json:"heading,omitempty"`
}

type relation struct {
	Kind      string  `json:"kind"`
	Direction string  `json:"direction"`
	Key       *string `json:"key,omitempty"`
	Number    string  `json:"number"`
	Title     string  `json:"title,omitempty"`
}

type node struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type payload struct {
	Number         string     `json:"number"`
	Title          string     `json:"title"`
	Type           string     `json:"type"`
	Issuer         string     `json:"issuer"`
	Issued         *string    `json:"issued"`
	HasText        bool       `json:"has_text"`
	TextComplete   bool       `json:"text_complete"`
	SourceURL      *string    `json:"source_url"`
	Relations      []relation `json:"relations"`
	RelationsTotal int        `json:"relations_total"`
	Nodes          []node     `json:"nodes"`
	Outline        []outline  `json:"outline"`
}

type status struct {
	Class       string `json:"class"`
	Current     bool   `json:"current"`
	Basis       string `json:"basis"`
	StatusBasis string `json:"status_basis"`
	Unresolved  bool   `json:"unresolved"`
}

type document struct {
	Key           string   `json:"key"`
	Number        string   `json:"number"`
	Title         string   `json:"title"`
	Type          string   `json:"type"`
	Issuer        string   `json:"issuer"`
	Issued        *string  `json:"issued"`
	EffectiveFrom *string  `json:"effective_from"`
	EffectiveTo   *string  `json:"effective_to"`
	Status        status   `json:"status"`
	GapKinds      []string `json:"gap_kinds"`
	HasText       bool     `json:"has_text"`
	TextComplete  bool     `json:"text_complete"`
	SourceURL     *string  `json:"source_url"`
}

type summaryResponse struct {
	Release        string     `json:"release"`
	Projection     int        `json:"projection"`
	StatusDate     string     `json:"status_date"`
	Index          bool       `json:"index"`
	Document       document   `json:"document"`
	Relations      []relation `json:"relations"`
	RelationsTotal int        `json:"relations_total"`
	Nodes          []node     `json:"nodes"`
	Outline        []outline  `json:"outline"`
}

type listResponse struct {
	Release    string `json:"release"`
	Projection int    `json:"projection"`
	Total      int    `json:"total"`
	Items      []Item `json:"items"`
}

func summaryBody(row Row, release Release, now time.Time) ([]byte, error) {
	if len(row.Payload) > 65536 {
		return nil, ErrUnavailable
	}
	var p payload
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode public summary: %w", err)
	}
	date := retrieval.Date{Year: now.Year(), Month: now.Month(), Day: now.Day()}
	validity, gaps := retrieval.DocumentValidity(retrieval.Document{StatusClass: row.StatusClass, StatusBasis: row.StatusBasis, IsCurrent: row.IsCurrent, ValidityConflict: row.ValidityConflict, EffectiveFrom: row.EffectiveFrom, EffectiveTo: row.EffectiveTo, DerivationGaps: row.GapKinds}, date)
	gapKinds := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		switch gap.Kind {
		case "missing_effective_date", "consolidated_status_unordered", "consolidated_status_unresolved":
			gapKinds = append(gapKinds, gap.Kind)
		case "validity_conflict":
		default:
			return nil, ErrUnavailable
		}
	}
	if p.SourceURL != nil {
		parsed, err := url.Parse(*p.SourceURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			p.SourceURL = nil
		}
	}
	if p.Outline == nil {
		p.Outline = []outline{}
	}
	if p.Relations == nil {
		p.Relations = []relation{}
	}
	if p.Nodes == nil {
		p.Nodes = []node{}
	}
	if !release.HeadingsApproved {
		for i := range p.Outline {
			p.Outline[i].Heading = ""
		}
	}
	response := summaryResponse{Release: release.ID, Projection: release.Projection, StatusDate: date.String(), Index: row.Indexed, Relations: p.Relations, RelationsTotal: p.RelationsTotal, Nodes: p.Nodes, Outline: p.Outline, Document: document{Key: row.Key, Number: p.Number, Title: p.Title, Type: p.Type, Issuer: p.Issuer, Issued: p.Issued, EffectiveFrom: dateString(row.EffectiveFrom), EffectiveTo: dateString(row.EffectiveTo), Status: status{Class: validity.StatusClass, Current: validity.Current, Basis: validity.Basis, StatusBasis: validity.StatusBasis, Unresolved: validity.Unresolved}, GapKinds: gapKinds, HasText: p.HasText, TextComplete: p.TextComplete, SourceURL: p.SourceURL}}
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode public summary: %w", err)
	}
	return raw, nil
}

func dateString(date *time.Time) *string {
	if date == nil {
		return nil
	}
	value := date.Format(time.DateOnly)
	return &value
}
