package vbpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
)

// LookupNumber returns observed candidates and raw evidence for a named number.
func (c *Client) LookupNumber(ctx context.Context, number string) (lookup.Result, []json.RawMessage, error) {
	const capPages = 100_001
	definition := map[string]any{"source": "vbpl", "type": "named", "option": "docNum", "mode": "exact", "level": "TRUNG_UONG", "number": number}
	query, err := json.Marshal(definition)
	if err != nil {
		return lookup.Result{}, nil, fmt.Errorf("encode lookup definition: %w", err)
	}
	b := lookup.Builder{QueryIdentity: source.NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", number), Query: query, Items: []lookup.Item{}, Pages: []lookup.Page{}}
	var bodies []json.RawMessage
	seen := map[string]bool{}
	for pageNumber := 1; pageNumber <= capPages; pageNumber++ {
		p, pageErr := c.List(ctx, ListQuery{Page: pageNumber, PageSize: c.lookupPageSize, Option: "docNum", Mode: "exact", Level: "TRUNG_UONG", Keyword: number})
		if pageErr != nil {
			b.Failed = true
			return b.Result(), bodies, fmt.Errorf("lookup page %d: %w", pageNumber, pageErr)
		}
		bodies = append(bodies, p.Raw)
		sum := sha256.Sum256(p.Raw)
		record := lookup.Page{Ordinal: &pageNumber, SHA256: hex.EncodeToString(sum[:]), ResultIDs: []string{}}
		if p.totalKnown {
			total := p.Total
			record.Total = &total
			if b.TotalReported == nil {
				b.TotalReported = &total
			}
		} else {
			b.Uncertain = true
		}
		added := 0
		for _, item := range p.Items {
			record.ResultIDs = append(record.ResultIDs, item.ID)
			if !seen[item.ID] {
				seen[item.ID] = true
				added++
				b.Items = append(b.Items, lookup.Item{Source: "vbpl", SourceID: item.ID, RawNumber: present(item.DocNum), Title: present(item.Title), Issuer: present(item.AgencyName), IssueDate: present(item.IssueDate), AgencyIDs: item.AgencyIDs, PageRef: record.SHA256})
			}
		}
		b.Pages = append(b.Pages, record)
		if len(seen) > 100_000 {
			b.Failed = true
			return b.Result(), bodies, fmt.Errorf("lookup exceeds result limit")
		}
		if len(p.Items) == 0 || (b.TotalReported != nil && len(seen) >= *b.TotalReported) {
			b.EndedNormally = true
			return b.Result(), bodies, nil
		}
		if added == 0 {
			b.Uncertain = true
			return b.Result(), bodies, nil
		}
	}
	b.Failed = true
	return b.Result(), bodies, fmt.Errorf("lookup exceeds page limit")
}
func present(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
