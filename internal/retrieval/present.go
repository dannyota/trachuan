package retrieval

import (
	"context"
	"fmt"
)

// DefaultLimit returns the search limit for the requested detail.
func DefaultLimit(d Detail) int {
	if d == DetailFull {
		return FullDefaultLimit
	}
	return CompactDefaultLimit
}

// Present loads document metadata and presents search evidence.
func (e *Engine) Present(ctx context.Context, resp Response, d Detail) (SearchResult, error) {
	seen := make(map[string]bool)
	var keys []string
	for _, hits := range [][]Hit{resp.Hits, resp.NonCurrent} {
		for _, h := range hits {
			if !seen[h.Identity.DocumentKey] {
				seen[h.Identity.DocumentKey] = true
				keys = append(keys, h.Identity.DocumentKey)
			}
		}
	}
	docs, err := e.Documents(ctx, keys)
	if err != nil {
		return SearchResult{}, fmt.Errorf("load presentation documents: %w", err)
	}
	return present(resp, docs, d), nil
}

func present(resp Response, docs map[string]DocumentEntry, d Detail) SearchResult {
	result := SearchResult{Release: resp.Release, Abstain: resp.Abstain, Reason: resp.Reason, Message: resp.Message, Gaps: resp.Gaps, Documents: make(map[string]DocumentEntry)}
	seen := make(map[string]bool)
	convert := func(hits []Hit) []PresentedHit {
		var presented []PresentedHit
		for _, h := range hits {
			key := h.Identity.DocumentKey
			if !seen[key] {
				seen[key] = true
				if doc, ok := docs[key]; ok {
					doc.Status = Status{Class: h.Stamp.StatusClass, Basis: h.Stamp.Basis, StatusBasis: h.Stamp.StatusBasis, Current: h.Stamp.Current}
					result.Documents[key] = doc
				}
			}
			p := PresentedHit{Handle: Handle(h.Identity), Doc: key, Citation: h.Citation, Gaps: h.Gaps, Warnings: warningPointers(h)}
			for _, node := range h.Path {
				p.Path = append(p.Path, PresentedPathNode{Label: node.Label, Heading: node.Heading, Handle: Handle(node.Identity)})
			}
			if d == DetailFull {
				p.Text, p.Fragments, p.Relations, p.Complete = h.Text, h.Fragments, h.Relations, h.Complete
				score := h.Score
				p.Score = &score
				if !h.Complete {
					p.Next = encodeNext(cursor{V: 1, Release: resp.Release, Item: ReadItem{Handle: p.Handle}})
				}
			} else {
				p.Snippet, p.Complete = Snippet(h, SnippetCap)
			}
			presented = append(presented, p)
		}
		return presented
	}
	result.Hits = convert(resp.Hits)
	result.NonCurrent = convert(resp.NonCurrent)
	return result
}

func warningPointers(h Hit) []Pointer {
	var warnings []Pointer
	for _, r := range h.Relations {
		switch r.Kind {
		case "repeals", "replaces", "suspends", "amends_supplements":
		default:
			continue
		}
		if r.TargetKey != nil && *r.TargetKey == h.Identity.DocumentKey && r.TargetAnchor != nil {
			p := Pointer{Dir: "in", Kind: r.Kind, Key: r.SourceKey, Number: r.SourceNumber, InRelease: r.SourceResolved}
			if p.InRelease {
				p.Handle = p.Key
			}
			warnings = append(warnings, p)
		}
		if r.SourceKey == h.Identity.DocumentKey && r.SourceAnchor != nil {
			p := Pointer{Dir: "out", Kind: r.Kind, Number: r.TargetNumber, InRelease: r.TargetResolved}
			if r.TargetKey != nil {
				p.Key = *r.TargetKey
			}
			if p.InRelease {
				p.Handle = p.Key
			}
			warnings = append(warnings, p)
		}
	}
	return warnings
}
