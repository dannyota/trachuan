package textparse

import (
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/anchor"
)

// Parse builds accepted articles and their checked clause and point descendants.
func Parse(paragraphs []Paragraph) Result {
	r := ParseArticles(paragraphs)
	if len(r.Provisions) == 0 {
		return r
	}
	counts := &StructureCounts{Reasons: make(map[string]int)}
	r.Counts.Structure = counts
	quoted, _ := quotedParagraphs(paragraphs)
	indices := make(map[int]int, len(paragraphs))
	for i, p := range paragraphs {
		indices[p.Ordinal] = i
	}
	grammar := newChildGrammar()
	var provisions []Provision
	var ancestors [3]string
	scope := -2
	for _, article := range r.Provisions {
		if article.ScopeIndex != scope {
			ancestors = [3]string{}
			scope = article.ScopeIndex
		}
		setStructureParent(&article, &ancestors)
		provisions = append(provisions, article)
		if article.Kind != "dieu" {
			continue
		}
		start, end := indices[article.StartOrdinal], indices[article.EndOrdinal]+1
		children, tails, reason := splitArticle(article, paragraphs[start:end], quoted[start:end], grammar)
		if reason != "" {
			counts.Reasons[reason]++
			continue
		}
		if len(children) > 0 {
			counts.ArticlesSplit++
		}
		counts.Reasons["point_tail"] += tails
		for _, p := range children {
			if p.Kind == "khoan" {
				counts.Clauses++
			} else {
				counts.Points++
			}
		}
		provisions = append(provisions, children...)
	}
	if counts.Reasons["point_tail"] == 0 {
		delete(counts.Reasons, "point_tail")
	}
	r.Provisions = provisions
	return r
}

type childStart struct {
	index              int
	kind, token, style string
}

func splitArticle(article Provision, paragraphs []Paragraph, quoted []bool, grammar childGrammar) ([]Provision, int, string) {
	var clauses, points []childStart
	for i := 1; i < len(paragraphs); i++ {
		if quoted[i] {
			continue
		}
		start, reason := grammar.read(paragraphs[i].Text)
		if reason != "" {
			return nil, 0, reason
		}
		start.index = i
		switch start.kind {
		case "khoan":
			clauses = append(clauses, start)
		case "diem":
			points = append(points, start)
		}
	}
	if len(clauses) == 0 {
		children, tail, reason := splitPoints(article, paragraphs, points, true)
		return children, tail, reason
	}
	if len(points) > 0 && points[0].index < clauses[0].index {
		return nil, 0, "points_before_clauses"
	}
	if reason := checkChildren(clauses, false); reason != "" {
		return nil, 0, reason
	}
	if len(clauses) < 2 {
		return nil, 0, "single_clause"
	}
	var children []Provision
	tails, pointIndex := 0, 0
	for i, start := range clauses {
		end := len(paragraphs)
		if i+1 < len(clauses) {
			end = clauses[i+1].index
		}
		clause := childProvision(article, paragraphs[start.index:end], start, i+1)
		if !uniqueContainment(article.Text, clause.Text) {
			return nil, 0, "containment"
		}
		var ownPoints []childStart
		for pointIndex < len(points) && points[pointIndex].index < end {
			point := points[pointIndex]
			point.index -= start.index
			ownPoints = append(ownPoints, point)
			pointIndex++
		}
		descendants, tail, reason := splitPoints(clause, paragraphs[start.index:end], ownPoints, false)
		if reason != "" {
			return nil, 0, reason
		}
		tails += tail
		children = append(children, clause)
		children = append(children, descendants...)
	}
	return children, tails, ""
}

func splitPoints(parent Provision, paragraphs []Paragraph, points []childStart, direct bool) ([]Provision, int, string) {
	if len(points) == 0 {
		return nil, 0, ""
	}
	if reason := checkChildren(points, true); reason != "" {
		return nil, 0, reason
	}
	if direct && len(points) < 2 {
		return nil, 0, "point_sequence"
	}
	for _, p := range paragraphs[points[len(points)-1].index+1:] {
		if !subItem(p.Text) {
			return nil, 1, ""
		}
	}
	var children []Provision
	for i, start := range points {
		end := len(paragraphs)
		if i+1 < len(points) {
			end = points[i+1].index
		}
		point := childProvision(parent, paragraphs[start.index:end], start, i+1)
		if !uniqueContainment(parent.Text, point.Text) {
			return nil, 0, "containment"
		}
		children = append(children, point)
	}
	return children, 0, ""
}

func uniqueContainment(parent, child string) bool {
	if child == "" {
		return false
	}
	first := strings.Index(parent, child)
	// Advance from the match's start so a second, overlapping match also rejects it.
	return first >= 0 && !strings.Contains(parent[first+1:], child)
}

func childProvision(parent Provision, paragraphs []Paragraph, start childStart, ordinal int) Provision {
	label := "Khoản "
	if start.kind == "diem" {
		label = "Điểm "
	}
	p := makeProvision(paragraphs, start.kind, start.token, label+start.token, "", parent.Anchor+"/"+anchor.Segment(start.kind, start.token), parent.ScopeIndex)
	p.ParentAnchor = parent.Anchor
	p.Ordinal = ordinal
	return p
}

func subItem(text string) bool { return strings.ContainsRune("-+•–", []rune(text)[0]) }

func setStructureParent(p *Provision, ancestors *[3]string) {
	level := 3
	switch p.Kind {
	case "phan":
		level = 0
	case "chuong":
		level = 1
	case "muc":
		level = 2
	}
	for i := level - 1; i >= 0; i-- {
		if ancestors[i] != "" {
			p.ParentAnchor = ancestors[i]
			break
		}
	}
	if level < 3 {
		ancestors[level] = p.Anchor
		for i := level + 1; i < len(ancestors); i++ {
			ancestors[i] = ""
		}
	}
}
