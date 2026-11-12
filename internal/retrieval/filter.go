package retrieval

import (
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func normalizeFilters(req Request) (Request, error) {
	fields := []*[]string{&req.Nodes, &req.DocTypes, &req.Issuers, &req.Documents}
	for i, field := range fields {
		budget := 20
		if i == 1 || i == 2 {
			budget = 10
		}
		if len(*field) > budget {
			return Request{}, ErrInvalidRequest
		}
		values := make([]string, 0, len(*field))
		for _, value := range *field {
			if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
				return Request{}, ErrInvalidRequest
			}
			value = strings.TrimSpace(value)
			if i == 1 || i == 2 {
				value = norm.NFC.String(value)
			}
			if value == "" || i == 0 && !validTaxonomyCode(value) {
				return Request{}, ErrInvalidRequest
			}
			values = append(values, value)
		}
		slices.Sort(values)
		*field = slices.Compact(values)
	}
	return req, nil
}

func hasFilters(req Request) bool {
	return len(req.Nodes)+len(req.DocTypes)+len(req.Issuers)+len(req.Documents) > 0
}

func matchesFilter(req Request, id Identity, nodes []string, doc DocumentEntry) bool {
	if len(req.Documents) > 0 && !slices.Contains(req.Documents, id.DocumentKey) {
		return false
	}
	if len(req.DocTypes) > 0 && !slices.Contains(req.DocTypes, norm.NFC.String(strings.TrimSpace(doc.Type))) {
		return false
	}
	if len(req.Issuers) > 0 && !slices.Contains(req.Issuers, norm.NFC.String(strings.TrimSpace(doc.Issuer))) {
		return false
	}
	if len(req.Nodes) > 0 {
		for _, filter := range req.Nodes {
			for _, code := range nodes {
				if code == filter || strings.HasPrefix(code, filter+".") {
					return true
				}
			}
		}
		return false
	}
	return true
}

func validateServedFilters(req Request, refs map[string]NodeRef) error {
	for _, code := range req.Nodes {
		if _, ok := refs[code]; !ok {
			return ErrInvalidRequest
		}
	}
	return nil
}
