package mcpapi

import (
	"context"
	"strconv"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type statusResult struct {
	Release          string           `json:"release"`
	Documents        int64            `json:"documents"`
	Provisions       int64            `json:"provisions"`
	Nodes            int64            `json:"nodes"`
	Gaps             map[string]int64 `json:"gaps,omitempty"`
	NewestSourceDate string           `json:"newest_source_date,omitempty"`
}

func (cfg Config) status(ctx context.Context, in countryInput) (statusResult, error) {
	lease, err := cfg.resolve(ctx, in.Country, "")
	if err != nil {
		return statusResult{}, err
	}
	defer lease.Done()
	status, err := lease.Engine.Status(ctx)
	if err != nil {
		return statusResult{}, err
	}
	result := statusResult{Release: lease.Release, Documents: status.Documents, Provisions: status.Provisions, Nodes: status.Nodes, Gaps: status.Gaps}
	if status.NewestSourceDate != nil {
		result.NewestSourceDate = status.NewestSourceDate.Format(time.DateOnly)
	}
	return result, nil
}

type gapsInput struct {
	Country string `json:"country"`
	Kind    string `json:"kind,omitempty"`
	Cursor  string `json:"cursor,omitempty"`
}
type gapsResult struct {
	Release    string          `json:"release"`
	Gaps       []retrieval.Gap `json:"gaps,omitempty"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

func (cfg Config) gaps(ctx context.Context, in gapsInput) (gapsResult, error) {
	var after int64
	if in.Cursor != "" {
		var err error
		after, err = strconv.ParseInt(in.Cursor, 10, 64)
		if err != nil || after < 0 {
			return gapsResult{}, invalidRequest{}
		}
	}
	lease, err := cfg.resolve(ctx, in.Country, "")
	if err != nil {
		return gapsResult{}, err
	}
	defer lease.Done()
	gaps, next, err := lease.Engine.Gaps(ctx, in.Kind, after, retrieval.GapPageSize)
	if err != nil {
		return gapsResult{}, err
	}
	result := gapsResult{Release: lease.Release, Gaps: gaps}
	if next != 0 {
		result.NextCursor = strconv.FormatInt(next, 10)
	}
	return result, nil
}
