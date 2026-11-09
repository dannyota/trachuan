package chunk

import (
	"fmt"
	"sort"
	"unicode"
)

func mappedSpans(t textRun, offset int, role string) []Span {
	var spans []Span
	active := map[string]int{}
	for i, r := range t {
		for _, p := range r.positions {
			idx, ok := active[p.id]
			if ok && spans[idx].End == p.offset && spans[idx].ContentEnd == offset+i {
				spans[idx].End++
				spans[idx].ContentEnd++
				continue
			}
			spans = append(spans, Span{NodeID: p.id, Start: p.offset, End: p.offset + 1, ContentStart: offset + i, ContentEnd: offset + i + 1, Role: role})
			active[p.id] = len(spans) - 1
		}
	}
	return spans
}
func (s *splitter) omit(t textRun, reason string, kept *int) {
	for _, span := range mappedSpans(t, 0, "body") {
		s.coverage.Omissions = append(s.coverage.Omissions, Omission{NodeID: span.NodeID, Start: span.Start, End: span.End, Reason: reason, KeptOrdinal: kept})
	}
}
func (s *splitter) finalCoverage(nodes []Node) {
	covered := map[string][]bool{}
	for _, n := range nodes {
		covered[n.NodeID] = make([]bool, len([]rune(n.Text)))
	}
	for _, c := range s.file.Chunks {
		for _, sp := range c.Spans {
			for i := sp.Start; i < sp.End; i++ {
				covered[sp.NodeID][i] = true
			}
		}
	}
	for _, n := range nodes {
		r := []rune(n.Text)
		flags := covered[n.NodeID]
		for i := 0; i < len(r); {
			if !flags[i] {
				i++
				continue
			}
			start := i
			for i < len(r) && flags[i] {
				i++
			}
			s.coverage.CoveredRanges = append(s.coverage.CoveredRanges, Range{n.NodeID, start, i})
		}
		for i := 0; i < len(r); {
			if !unicode.IsSpace(r[i]) {
				i++
				continue
			}
			start := i
			for i < len(r) && unicode.IsSpace(r[i]) {
				i++
			}
			s.coverage.WhitespaceRanges = append(s.coverage.WhitespaceRanges, Range{n.NodeID, start, i})
		}
	}
	sort.SliceStable(s.coverage.CoveredRanges, func(i, j int) bool {
		a, b := s.coverage.CoveredRanges[i], s.coverage.CoveredRanges[j]
		if a.NodeID != b.NodeID {
			return a.NodeID < b.NodeID
		}
		return a.Start < b.Start
	})
}

func (s *splitter) checkCoverage(nodes []Node) error {
	accounted := make(map[string][]bool, len(nodes))
	for _, n := range nodes {
		accounted[n.NodeID] = make([]bool, len([]rune(n.Text)))
	}
	for _, r := range s.coverage.CoveredRanges {
		for i := r.Start; i < r.End; i++ {
			accounted[r.NodeID][i] = true
		}
	}
	for _, r := range s.coverage.Omissions {
		for i := r.Start; i < r.End; i++ {
			accounted[r.NodeID][i] = true
		}
	}
	for _, n := range nodes {
		for i, r := range []rune(n.Text) {
			if !unicode.IsSpace(r) && !accounted[n.NodeID][i] {
				return fmt.Errorf("unaccounted source interval at %s offset %d", n.NodeID, i)
			}
		}
	}
	return nil
}
