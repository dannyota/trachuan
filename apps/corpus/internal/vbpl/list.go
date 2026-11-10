package vbpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// List requests one page of a discovery query.
func (c *Client) List(ctx context.Context, q ListQuery) (ListPage, error) {
	input := struct {
		Page      int      `json:"pageNumber"`
		Size      int      `json:"pageSize"`
		Sort      string   `json:"sortBy"`
		Direction string   `json:"sortDirection"`
		Group     bool     `json:"groupVbpl"`
		Level     string   `json:"agencyLevel"`
		Option    string   `json:"optionDoc"`
		Mode      string   `json:"matchMode"`
		Agencies  []string `json:"agencyIds,omitempty"`
		Keyword   string   `json:"keyword,omitempty"`
	}{q.Page, 50, "issueDate", "desc", false, "TRUNG_UONG", "title", "all_words", q.AgencyIDs, q.Keyword}
	if q.PageSize != 0 {
		input.Size = q.PageSize
	}
	if q.Option != "" {
		input.Option = q.Option
	}
	if q.Mode != "" {
		input.Mode = q.Mode
	}
	if q.Level != "" {
		input.Level = q.Level
	}
	body, err := json.Marshal(input)
	if err != nil {
		return ListPage{}, fmt.Errorf("encode list query: %w", err)
	}
	raw, err := c.api(ctx, http.MethodPost, "/doc/all", body)
	if err != nil {
		return ListPage{}, err
	}
	var result struct {
		Total *int              `json:"total"`
		Items []json.RawMessage `json:"items"`
	}
	if err := data(raw, &result); err != nil {
		return ListPage{}, err
	}
	page := ListPage{Raw: raw}
	if result.Total != nil {
		page.Total = *result.Total
	}
	page.totalKnown = result.Total != nil
	for _, item := range result.Items {
		var v struct {
			ID, DocNum, Title, IssueDate, AgencyName string
			AgencyIDs                                []string
		}
		if err := json.Unmarshal(item, &v); err != nil {
			return ListPage{}, fmt.Errorf("decode list item: %w", err)
		}
		page.Items = append(page.Items, ListItem{ID: v.ID, DocNum: v.DocNum, Title: v.Title, IssueDate: v.IssueDate, Raw: item, AgencyIDs: v.AgencyIDs, AgencyName: v.AgencyName})
	}
	return page, nil
}
