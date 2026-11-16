package build

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type summaryOutline struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Anchor  string `json:"anchor"`
	Heading string `json:"heading,omitempty"`
}

type summaryRelation struct {
	Kind      string  `json:"kind"`
	Direction string  `json:"direction"`
	Key       *string `json:"key,omitempty"`
	Number    string  `json:"number"`
	Title     string  `json:"title,omitempty"`
}

type summaryNode struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type summaryPayload struct {
	Number         string            `json:"number"`
	Title          string            `json:"title"`
	Type           string            `json:"type"`
	Issuer         string            `json:"issuer"`
	Issued         *string           `json:"issued"`
	HasText        bool              `json:"has_text"`
	TextComplete   bool              `json:"text_complete"`
	SourceURL      *string           `json:"source_url"`
	Relations      []summaryRelation `json:"relations"`
	RelationsTotal int               `json:"relations_total"`
	Nodes          []summaryNode     `json:"nodes"`
	Outline        []summaryOutline  `json:"outline"`
}

type summaryDocument struct {
	Key              string         `json:"document_key"`
	Class            string         `json:"class"`
	Projection       int            `json:"projection"`
	Indexed          bool           `json:"indexed"`
	StatusClass      string         `json:"status_class"`
	StatusBasis      string         `json:"status_basis"`
	IsCurrent        bool           `json:"is_current"`
	ValidityConflict bool           `json:"validity_conflict"`
	EffFrom          *time.Time     `json:"eff_from"`
	EffTo            *time.Time     `json:"eff_to"`
	GapKinds         []string       `json:"gap_kinds"`
	Payload          summaryPayload `json:"payload"`
	TextSource       string         `json:"-"`
}

func structuralSummaryGap(kind string) bool {
	switch kind {
	case "ambiguous_tree_text", "article_number_out_of_range", "article_sequence", "structure_uncertain", "text_number_mismatch", "tree_headings_only", "tree_headings_partial", "tree_text_partial", "unknown_node_kind", "invalid_ancestry", "no_tree", "text_source_ambiguous":
		return true
	default:
		return false
	}
}

func makeSummary(d summaryDocument, gaps []InventoryGap, excluded map[string]bool) (summaryDocument, bool, error) {
	d.Projection = 1
	d.GapKinds = []string{}
	headings := d.TextSource == "source_tree"
	for _, g := range gaps {
		if g.ItemKey != d.Key {
			continue
		}
		if structuralSummaryGap(g.Kind) {
			headings = false
		}
		if g.Kind == "consolidated_status_unordered" || g.Kind == "consolidated_status_unresolved" {
			d.GapKinds = append(d.GapKinds, g.Kind)
		}
	}
	slices.Sort(d.GapKinds)
	d.GapKinds = slices.Compact(d.GapKinds)
	p := &d.Payload
	p.Number, p.Title, p.Type, p.Issuer = normalizeLabel(p.Number), normalizeLabel(p.Title), normalizeLabel(p.Type), normalizeLabel(p.Issuer)
	for _, f := range []struct {
		name, value string
		cap         int
	}{{"number", p.Number, 200}, {"title", p.Title, 1000}, {"type", p.Type, 200}, {"issuer", p.Issuer, 200}} {
		if utf8.RuneCountInString(f.value) > f.cap {
			return summaryDocument{}, false, fmt.Errorf("summary %s: %s exceeds %d characters", d.Key, f.name, f.cap)
		}
	}
	for i := range p.Relations {
		r := &p.Relations[i]
		r.Number, r.Title = normalizeLabel(r.Number), normalizeLabel(r.Title)
		if utf8.RuneCountInString(r.Title) > 1000 || utf8.RuneCountInString(r.Number) > 200 {
			return summaryDocument{}, false, fmt.Errorf("summary %s: relation identity exceeds cap", d.Key)
		}
	}
	for i := range p.Nodes {
		p.Nodes[i].Name = normalizeLabel(p.Nodes[i].Name)
		if utf8.RuneCountInString(p.Nodes[i].Name) > 200 {
			return summaryDocument{}, false, fmt.Errorf("summary %s: topic name exceeds 200 characters", d.Key)
		}
	}
	p.RelationsTotal = len(p.Relations)
	p.Relations = append([]summaryRelation{}, p.Relations[:min(50, len(p.Relations))]...)
	p.Nodes = append([]summaryNode{}, p.Nodes[:min(10, len(p.Nodes))]...)
	outline := make([]summaryOutline, 0, len(p.Outline))
	for _, n := range p.Outline {
		if n.Kind != "phan" && n.Kind != "chuong" && n.Kind != "muc" && n.Kind != "dieu" {
			continue
		}
		if !headings || (n.Kind != "chuong" && n.Kind != "dieu") || utf8.RuneCountInString(n.Heading) > 300 || excluded[n.Anchor] {
			n.Heading = ""
		}
		outline = append(outline, n)
	}
	capped := len(outline) > 1500
	d.Indexed = false
	if capped {
		outline = []summaryOutline{}
	}
	for _, n := range outline {
		if d.Class == "primary" && strings.TrimSpace(n.Label) != "" {
			d.Indexed = true
		}
	}
	p.Outline = outline
	b, err := json.Marshal(d)
	if err != nil {
		return summaryDocument{}, false, fmt.Errorf("encode summary: %w", err)
	}
	if len(b) > 65536 {
		return summaryDocument{}, false, fmt.Errorf("summary %s: serialized row exceeds 65536 octets", d.Key)
	}
	return d, capped, nil
}
