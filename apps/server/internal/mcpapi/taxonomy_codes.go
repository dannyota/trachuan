package mcpapi

import (
	"context"

	"danny.vn/trachuan/internal/retrieval"
)

type taxonomyCodesEngine interface {
	TaxonomyCodes(context.Context) ([]string, error)
}

type TaxonomyCodesResult struct {
	Release string   `json:"release"`
	Codes   []string `json:"codes"`
}

func (o *Operations) TaxonomyCodes(ctx context.Context) (TaxonomyCodesResult, error) {
	lease, err := o.cfg.resolve(ctx, "vn", "")
	if err != nil {
		return TaxonomyCodesResult{}, retrieval.ErrUnavailable
	}
	defer lease.Done()
	engine, ok := lease.Engine.(taxonomyCodesEngine)
	if !ok || lease.Release == "" {
		return TaxonomyCodesResult{}, retrieval.ErrUnavailable
	}
	codes, err := engine.TaxonomyCodes(ctx)
	if err != nil || codes == nil {
		return TaxonomyCodesResult{}, retrieval.ErrUnavailable
	}
	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		if code == "" || code == "codes" || seen[code] {
			return TaxonomyCodesResult{}, retrieval.ErrUnavailable
		}
		seen[code] = true
	}
	return TaxonomyCodesResult{Release: lease.Release, Codes: codes}, nil
}
