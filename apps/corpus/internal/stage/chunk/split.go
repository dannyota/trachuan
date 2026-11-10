package chunk

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type splitter struct {
	file     File
	coverage Coverage
	meta     Meta
	recipe   Recipe
	seen     map[string]int
}

type carriedLead struct {
	owner *treeNode
	text  textRun
}

func leadText(leads []carriedLead) textRun {
	var text textRun
	for _, lead := range leads {
		text = join(text, lead.text)
	}
	return text
}

func (s *splitter) emitLeads(leads []carriedLead) {
	for _, lead := range leads {
		s.shards(lead.owner, nil, lead.text)
	}
}

func normalized(s string) string { return norm.NFC.String(collapse(s)) }
func labelOnly(t textRun, n Node) bool {
	clean := func(s string) string {
		return strings.TrimRight(strings.TrimSpace(strings.ToLower(norm.NFC.String(s))), ".:)")
	}
	return n.Label != "" && clean(plain(t)) == clean(n.Label)
}
func rough(t textRun) int {
	if len(t) == 0 {
		return 0
	}
	if len(t) < 2 {
		return 1
	}
	return len(t) / 2
}
func trimRun(t textRun) textRun {
	for len(t) > 0 && unicode.IsSpace(t[0].value) {
		t = t[1:]
	}
	for len(t) > 0 && unicode.IsSpace(t[len(t)-1].value) {
		t = t[:len(t)-1]
	}
	return t
}

// Split renders and splits extracted provisions while recording source coverage.
func Split(nodes []Node, meta Meta, recipe Recipe) (File, Coverage, error) {
	s := splitter{file: File{Version: 1, DocumentKey: meta.Key, Chunks: []Chunk{}}, coverage: Coverage{Version: 1, DocumentKey: meta.Key, Gaps: []Gap{}, Omissions: []Omission{}, CoveredRanges: []Range{}, WhitespaceRanges: []Range{}}, meta: meta, recipe: recipe, seen: make(map[string]int)}
	roots, err := tree(nodes)
	if err != nil {
		return s.file, s.coverage, err
	}
	if len(nodes) == 0 {
		s.coverage.Gaps = append(s.coverage.Gaps, Gap{Kind: "no_tree"})
		return s.file, s.coverage, nil
	}
	if recipe.BodyLimit <= 0 || recipe.MinimumRunes < 0 {
		return s.file, s.coverage, fmt.Errorf("invalid chunk recipe limits")
	}
	for _, root := range roots {
		render(root, &s.coverage)
		propagateAliases(root)
	}
	for _, root := range roots {
		s.outside(root)
	}
	if len(s.file.Chunks) == 0 {
		s.coverage.Gaps = append(s.coverage.Gaps, Gap{Kind: "no_chunks"})
	}
	if len(s.coverage.Omissions) > 0 {
		s.coverage.Gaps = append(s.coverage.Gaps, Gap{Kind: "chunk_omission", Detail: fmt.Sprintf("%d source intervals", len(s.coverage.Omissions))})
	}
	s.finalCoverage(nodes)
	if err := s.checkCoverage(nodes); err != nil {
		return File{}, Coverage{}, err
	}
	return s.file, s.coverage, nil
}

func (s *splitter) outside(n *treeNode) {
	if n.node.Kind == "dieu" {
		s.article(n, nil)
		return
	}
	for _, part := range n.own {
		if len(trimRun(part)) > 0 {
			s.shards(n, nil, part)
		}
	}
	for _, child := range n.children {
		s.outside(child)
	}
}

func legal(n *treeNode) bool {
	return n.node.Kind == "dieu" || n.node.Kind == "khoan" || n.node.Kind == "diem"
}

func (s *splitter) article(n *treeNode, leads []carriedLead) {
	if n.hasAmbiguous {
		for _, part := range n.own {
			s.shards(n, leads, part)
		}
		for _, child := range n.children {
			s.article(child, nil)
		}
		return
	}
	if rough(join(leadText(leads), n.rendered)) <= s.recipe.BodyLimit {
		s.emit(n, leadText(leads), n.rendered)
		return
	}
	var legalChildren []*treeNode
	for _, child := range n.children {
		if legal(child) {
			legalChildren = append(legalChildren, child)
		}
	}
	if len(legalChildren) == 0 {
		s.shards(n, leads, n.rendered)
		return
	}
	var ownLead textRun
	if len(n.own) > 0 {
		ownLead = trimRun(n.own[0])
	}
	if labelOnly(ownLead, n.node) {
		s.omit(ownLead, "label_only", nil)
		ownLead = nil
	}
	combined := append([]carriedLead(nil), leads...)
	if len(ownLead) > 0 {
		combined = append(combined, carriedLead{owner: n, text: ownLead})
	}
	lead := leadText(combined)
	leadEmitted := false
	if rough(lead) >= s.recipe.BodyLimit || !s.leadFits(lead, legalChildren) {
		s.emitLeads(combined)
		combined = nil
		leadEmitted = true
	}
	before := len(s.file.Chunks)
	partIndex := 1
	for _, child := range n.children {
		if legal(child) {
			s.article(child, combined)
		} else {
			s.shards(n, nil, child.rendered)
		}
		if len(n.own) > 1 && len(child.rendered) > 0 {
			s.shards(n, nil, n.own[partIndex])
			partIndex++
		}
	}
	if len(s.file.Chunks) == before && len(ownLead) > 0 && !leadEmitted {
		s.shards(n, leads, ownLead)
	}
}

func (s *splitter) leadFits(lead textRun, children []*treeNode) bool {
	if len(lead) == 0 {
		return true
	}
	for _, child := range children {
		text := child.rendered
		for i := 0; i < len(text); {
			for i < len(text) && unicode.IsSpace(text[i].value) {
				i++
			}
			start := i
			for i < len(text) && !unicode.IsSpace(text[i].value) {
				i++
			}
			if start < i && rough(join(lead, text[start:i])) > s.recipe.BodyLimit {
				return false
			}
		}
	}
	return true
}

func (s *splitter) shards(n *treeNode, leads []carriedLead, body textRun) {
	body = trimRun(body)
	if len(body) == 0 {
		return
	}
	lead := leadText(leads)
	if rough(join(lead, body)) <= s.recipe.BodyLimit {
		s.emit(n, lead, body)
		return
	}
	// Split long lines into words and preserve mappings for their source characters.
	var lines []textRun
	start := 0
	for i := 0; i <= len(body); i++ {
		if i == len(body) || body[i].value == '\n' {
			line := trimRun(body[start:i])
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	var current textRun
	flush := func() {
		if len(current) > 0 {
			s.emit(n, lead, current)
			current = nil
		}
	}
	for _, line := range lines {
		if rough(join(lead, line)) <= s.recipe.BodyLimit {
			candidate := join(current, line)
			if len(current) > 0 && rough(join(lead, candidate)) > s.recipe.BodyLimit {
				flush()
				candidate = line
			}
			current = candidate
			continue
		}
		flush()
		var words []textRun
		for i := 0; i < len(line); {
			for i < len(line) && unicode.IsSpace(line[i].value) {
				i++
			}
			start = i
			for i < len(line) && !unicode.IsSpace(line[i].value) {
				i++
			}
			if start < i {
				words = append(words, line[start:i])
			}
		}
		for _, word := range words {
			if len(lead) > 0 && rough(join(lead, word)) > s.recipe.BodyLimit {
				s.emitLeads(leads)
				leads = nil
				lead = nil
			}
			candidate := append(textRun(nil), current...)
			if len(candidate) > 0 {
				candidate = append(candidate, mappedRune{value: ' '})
			}
			candidate = append(candidate, word...)
			if len(current) > 0 && rough(join(lead, candidate)) > s.recipe.BodyLimit {
				flush()
				candidate = word
			}
			current = candidate
		}
		flush()
	}
	flush()
}

func (s *splitter) emit(n *treeNode, lead, body textRun) {
	body = trimRun(body)
	lead = trimRun(lead)
	if len(body) == 0 {
		return
	}
	if labelOnly(body, n.node) {
		reason := "label_only"
		if n.node.Kind == "phan" || n.node.Kind == "chuong" || n.node.Kind == "muc" {
			reason = "structural_heading"
		}
		s.omit(body, reason, nil)
		return
	}
	if len([]rune(normalized(plain(body)))) < s.recipe.MinimumRunes {
		s.omit(body, "short", nil)
		return
	}
	content := join(lead, body)
	key := normalized(plain(content))
	if ordinal, ok := s.seen[key]; ok {
		s.omit(body, "duplicate", &ordinal)
		return
	}
	ordinal := len(s.file.Chunks)
	s.seen[key] = ordinal
	spans := mappedSpans(lead, 0, "lead")
	offset := len(lead)
	if len(lead) > 0 {
		offset++
	}
	spans = append(spans, mappedSpans(body, offset, "body")...)
	s.file.Chunks = append(s.file.Chunks, Chunk{Ordinal: ordinal, NodeID: n.node.NodeID, Anchor: n.node.Path, ContextPrefix: prefix(s.meta, n, s.recipe), Content: plain(content), RoughTokens: rough(content), Spans: spans})
}
