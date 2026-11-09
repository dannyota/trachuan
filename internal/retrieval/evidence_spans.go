package retrieval

import (
	"errors"
	"sort"
	"unicode"
)

func renderSpans(content string, spans []sourceSpan, nodes map[string]Provision, role, nodeID string) (rendered, error) {
	selected := []sourceSpan{}
	for _, s := range spans {
		if s.Role == role && (nodeID == "" || s.NodeID == nodeID) {
			selected = append(selected, s)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].ContentStart != selected[j].ContentStart {
			return selected[i].ContentStart < selected[j].ContentStart
		}
		return selected[i].ContentEnd < selected[j].ContentEnd
	})
	out := rendered{}
	offsets := map[int]int{}
	contentRunes := []rune(content)
	last := -1
	sourceRunes := map[string][]rune{}
	for _, s := range selected {
		p, ok := nodes[s.NodeID]
		if !ok {
			return rendered{}, ErrCacheInsufficient
		}
		src, cached := sourceRunes[s.NodeID]
		if !cached {
			src = []rune(p.Text)
			sourceRunes[s.NodeID] = src
		}
		if s.Start < 0 || s.End < s.Start || s.End > len(src) || s.ContentStart < 0 || s.ContentEnd > len(contentRunes) || s.ContentEnd-s.ContentStart != s.End-s.Start {
			return rendered{}, errors.New("invalid source span")
		}
		if string(src[s.Start:s.End]) != string(contentRunes[s.ContentStart:s.ContentEnd]) {
			return rendered{}, errors.New("source span does not match content")
		}
		if last >= 0 && s.ContentStart > last {
			gap := []rune{' '}
			if s.ContentStart <= len(contentRunes) && last <= len(contentRunes) {
				gap = contentRunes[last:s.ContentStart]
				for _, r := range gap {
					if !unicode.IsSpace(r) {
						gap = []rune{'\n'}
						break
					}
				}
			}
			out.text = append(out.text, gap...)
		}
		textStart := -1
		for i := 0; i < s.End-s.Start; i++ {
			pos := s.ContentStart + i
			off, exists := offsets[pos]
			if !exists {
				off = len(out.text)
				offsets[pos] = off
				out.text = append(out.text, src[s.Start+i])
			}
			if textStart < 0 {
				textStart = off
			}
		}
		if textStart >= 0 {
			f := Fragment{Identity: p.Identity, Start: s.Start, End: s.End, TextStart: textStart, TextEnd: textStart + s.End - s.Start, Role: role, Complete: true}
			n := len(out.fragments)
			if n > 0 && out.fragments[n-1].Identity == f.Identity && out.fragments[n-1].End == f.Start && out.fragments[n-1].TextEnd == f.TextStart {
				out.fragments[n-1].End = f.End
				out.fragments[n-1].TextEnd = f.TextEnd
			} else {
				out.fragments = append(out.fragments, f)
			}
		}
		last = max(last, s.ContentEnd)
	}
	sort.SliceStable(out.fragments, func(i, j int) bool {
		a, b := out.fragments[i], out.fragments[j]
		if a.Identity.Country != b.Identity.Country {
			return a.Identity.Country < b.Identity.Country
		}
		if a.Identity.DocumentKey != b.Identity.DocumentKey {
			return a.Identity.DocumentKey < b.Identity.DocumentKey
		}
		if a.Identity.Anchor != b.Identity.Anchor {
			return a.Identity.Anchor < b.Identity.Anchor
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.TextStart < b.TextStart
	})
	merged := make([]Fragment, 0, len(out.fragments))
	for _, f := range out.fragments {
		n := len(merged)
		if n > 0 && merged[n-1].Identity == f.Identity && merged[n-1].Role == f.Role && merged[n-1].End == f.Start && merged[n-1].TextEnd == f.TextStart {
			merged[n-1].End = f.End
			merged[n-1].TextEnd = f.TextEnd
		} else {
			merged = append(merged, f)
		}
	}
	out.fragments = merged
	return out, nil
}
