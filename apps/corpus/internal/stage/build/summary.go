package build

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
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
	CappedFields     []string       `json:"-"`
}

func makeSummary(ctx context.Context, db summarySizer, d summaryDocument, gaps []InventoryGap, excluded map[string]bool) (summaryDocument, summaryCapStep, error) {
	d.Projection = 1
	d.GapKinds = []string{}
	headings := d.TextSource == "source_tree"
	for _, g := range gaps {
		structural, err := structuralSummaryGap(g.Kind)
		if err != nil {
			return summaryDocument{}, summaryWhole, fmt.Errorf("summary %s: %w", d.Key, err)
		}
		if g.ItemKey != d.Key {
			continue
		}
		if structural {
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
			d.CappedFields = append(d.CappedFields, f.name)
		}
	}
	if len(d.CappedFields) != 0 {
		return d, summaryFieldDropped, nil
	}
	p.RelationsTotal = len(p.Relations)
	relations := make([]summaryRelation, 0, min(50, len(p.Relations)))
	for _, r := range p.Relations {
		r.Number, r.Title = normalizeLabel(r.Number), normalizeLabel(r.Title)
		numberCapped := utf8.RuneCountInString(r.Number) > 200
		titleCapped := utf8.RuneCountInString(r.Title) > 1000
		if numberCapped {
			d.CappedFields = append(d.CappedFields, "relation.number")
		}
		if titleCapped {
			d.CappedFields = append(d.CappedFields, "relation.title")
		}
		if !numberCapped && !titleCapped && len(relations) < 50 {
			relations = append(relations, r)
		}
	}
	nodes := make([]summaryNode, 0, min(10, len(p.Nodes)))
	for _, n := range p.Nodes {
		n.Name = normalizeLabel(n.Name)
		if utf8.RuneCountInString(n.Name) > 600 {
			d.CappedFields = append(d.CappedFields, "topic.name")
			continue
		}
		if len(nodes) < 10 {
			nodes = append(nodes, n)
		}
	}
	p.Relations, p.Nodes = relations, nodes
	slices.Sort(d.CappedFields)
	d.CappedFields = slices.Compact(d.CappedFields)
	headings = headings && chapterSequenceOK(p.Outline)
	outline := make([]summaryOutline, 0, len(p.Outline))
	for _, n := range p.Outline {
		if n.Kind != "phan" && n.Kind != "chuong" && n.Kind != "muc" && n.Kind != "dieu" {
			continue
		}
		if !headings || (n.Kind != "chuong" && n.Kind != "dieu") || utf8.RuneCountInString(n.Heading) > 300 || headingShape(n.Heading) != "" || excluded[n.Anchor] {
			n.Heading = ""
		}
		outline = append(outline, n)
	}
	d.Indexed = false
	for _, n := range outline {
		if d.Class == "primary" && strings.TrimSpace(n.Label) != "" {
			d.Indexed = true
		}
	}
	p.Outline = outline
	return capSummary(ctx, db, d)
}

func chapterSequenceOK(outline []summaryOutline) bool {
	previous := 0
	for _, n := range outline {
		if n.Kind != "chuong" {
			continue
		}
		number, ok := chapterNumber(n.Label)
		if !ok || (number != previous+1 && number != 1) {
			return false
		}
		previous = number
	}
	return true
}

func chapterNumber(label string) (int, bool) {
	label = strings.ToLower(strings.TrimSpace(label))
	label = strings.TrimPrefix(label, "chương")
	label = strings.TrimRight(strings.TrimSpace(label), ".")
	if label == "" {
		return 0, false
	}
	runes := []rune(label)
	value := 0
	allDigits := true
	for _, r := range runes {
		digit := -1
		for _, span := range unicode.Digit.R16 {
			if r >= rune(span.Lo) && r <= rune(span.Hi) && (r-rune(span.Lo))%rune(span.Stride) == 0 {
				digit = int((r-rune(span.Lo))/rune(span.Stride)) % 10
				break
			}
		}
		for _, span := range unicode.Digit.R32 {
			if r >= rune(span.Lo) && r <= rune(span.Hi) && (r-rune(span.Lo))%rune(span.Stride) == 0 {
				digit = int((r-rune(span.Lo))/rune(span.Stride)) % 10
				break
			}
		}
		if digit < 0 {
			allDigits = false
			break
		}
		if value > (int(^uint(0)>>1)-digit)/10 {
			return 0, false
		}
		value = value*10 + digit
	}
	if allDigits {
		return value, true
	}
	roman := func(r rune) int {
		switch r {
		case 'i':
			return 1
		case 'v':
			return 5
		case 'x':
			return 10
		case 'l':
			return 50
		case 'c':
			return 100
		case 'd':
			return 500
		case 'm':
			return 1000
		}
		return 0
	}
	value = 0
	for i, r := range runes {
		v := roman(r)
		if v == 0 {
			return 0, false
		}
		if i+1 < len(runes) && roman(runes[i+1]) > v {
			value -= v
		} else {
			value += v
		}
	}
	return value, true
}

func headingShape(heading string) string {
	runes := []rune(strings.TrimSpace(heading))
	if len(runes) == 0 {
		return ""
	}
	if strings.ContainsRune(".:;?!", runes[len(runes)-1]) {
		return "end_mark"
	}
	for i := 0; i+2 < len(runes); i++ {
		if (runes[i] == '.' || runes[i] == ':') && unicode.IsSpace(runes[i+1]) {
			return "internal_break"
		}
	}
	if strings.ContainsRune("-+•*–—", runes[0]) {
		return "leading_marker"
	}
	return ""
}
