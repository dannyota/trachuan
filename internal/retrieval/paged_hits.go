package retrieval

import "fmt"

// PagedHits returns consecutive search pages from already collected arms.
func PagedHits(arms Arms, recipe Recipe, req Request, pageLimit, maxHits int) ([]Response, error) {
	if pageLimit < 1 || pageLimit > MaxLimit || maxHits < 1 || req.Cursor != "" || len(req.DocTypes)+len(req.Issuers)+len(req.Documents) != 0 {
		return nil, ErrInvalidRequest
	}
	var err error
	req, err = normalizeFilters(req)
	if err != nil {
		return nil, err
	}
	req.Limit = pageLimit
	walk, err := buildSearchWalk(arms, recipe, pageLimit, req.Arms)
	if err != nil {
		return nil, fmt.Errorf("plan paged hits: %w", err)
	}
	walk.filter(req, arms.EffectiveNodes, nil)
	c := searchCursor{V: 1, Release: arms.Release, FirstLimit: pageLimit, AsOf: arms.Date, Digest: requestDigest(req)}
	var pages []Response
	count := 0
	for {
		page, pageErr := walk.page(arms, recipe, req, c)
		if pageErr != nil {
			return nil, fmt.Errorf("shape paged hits: %w", pageErr)
		}
		pages = append(pages, page)
		count += len(page.Hits) + len(page.NonCurrent)
		if count >= maxHits || page.NextCursor == "" {
			break
		}
		c, err = decodeSearchCursor(page.NextCursor)
		if err != nil {
			return nil, fmt.Errorf("decode paged hits cursor: %w", err)
		}
		req.Cursor = page.NextCursor
	}
	return pages, nil
}
