package serve

import (
	"slices"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/internal/frameworks/businesskey"
	"danny.vn/trachuan/internal/retrieval"
	"golang.org/x/text/unicode/norm"
)

func (e *Engine) validate(r Request) (Request, error) {
	if !utf8.ValidString(r.Query) || utf8.RuneCountInString(r.Query) > 2000 || strings.TrimSpace(r.Query) == "" || strings.ContainsRune(r.Query, 0) || r.Limit < 0 || r.Limit > retrieval.MaxLimit || (r.Mode != "" && r.Mode != "exact") {
		return Request{}, ErrInvalidRequest
	}
	if r.Release != "" && r.Release != e.release {
		return Request{}, ErrReleaseExpired
	}
	if r.Limit == 0 {
		r.Limit = retrieval.CompactDefaultLimit
	}
	r.Mode = "exact"
	filters := make(map[string][]string)
	for key, values := range r.Filters {
		if key != "frameworks" && key != "versions" {
			return Request{}, ErrInvalidRequest
		}
		for _, value := range values {
			found := false
			for _, v := range e.versions {
				if (key == "frameworks" && value == v.info.Code) || (key == "versions" && value == v.handle) {
					found = true
					break
				}
			}
			if !found {
				return Request{}, ErrInvalidRequest
			}
		}
		if len(values) > 0 {
			values = slices.Clone(values)
			slices.Sort(values)
			filters[key] = slices.Compact(values)
		}
	}
	r.Filters = filters
	return r, nil
}

func (e *Engine) selectVersions(r Request) []int64 {
	ids := []int64{}
	for _, v := range e.versions {
		if f := r.Filters["frameworks"]; len(f) > 0 && !slices.Contains(f, v.info.Code) {
			continue
		}
		if f := r.Filters["versions"]; len(f) > 0 && !slices.Contains(f, v.handle) {
			continue
		}
		ids = append(ids, v.id)
	}
	return ids
}

type citationRoute struct{ code, citation string }

func (e *Engine) citations(query string) []citationRoute {
	q := strings.ToUpper(strings.TrimSpace(norm.NFC.String(query)))
	validator := businesskey.New()
	var routes []citationRoute
	seen := map[citationRoute]bool{}
	for _, alias := range []bool{false, true} {
		for _, v := range e.versions {
			names := []string{v.info.Code}
			if alias {
				names = v.aliases
			}
			for _, name := range names {
				prefix := strings.ToUpper(name) + " "
				if strings.HasPrefix(q, prefix) {
					citation := strings.TrimSpace(strings.TrimPrefix(q, prefix))
					if validator.Valid(v.info.Code, v.info.Version, citation) {
						route := citationRoute{v.info.Code, strings.ReplaceAll(citation, " ", "")}
						if !seen[route] {
							routes = append(routes, route)
							seen[route] = true
						}
					}
				}
			}
		}
	}
	return routes
}
