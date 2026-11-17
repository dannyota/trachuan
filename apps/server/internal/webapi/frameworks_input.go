package webapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
)

func frameworkInput(r *http.Request, operation, handle string) (json.RawMessage, error) {
	if operation == "search" {
		if r.URL.RawQuery != "" {
			return nil, invalidRequest()
		}
		return requestBody(r)
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, invalidRequest()
	}
	allowed := []string{"release"}
	switch operation {
	case "read":
		allowed = append(allowed, "next")
	case "mappings":
		allowed = append(allowed, "handle", "from", "to", "relation", "limit", "cursor")
	case "quality_gaps":
		allowed = append(allowed, "versions", "kind", "cursor")
	}
	in := map[string]any{"corpus": "frameworks"}
	for name, values := range query {
		if !slices.Contains(allowed, name) || len(values) == 0 {
			return nil, invalidRequest()
		}
		for _, value := range values {
			if value == "" {
				return nil, invalidRequest()
			}
		}
		switch name {
		case "relation", "versions":
			in[name] = values
		default:
			if len(values) != 1 {
				return nil, invalidRequest()
			}
			if name == "limit" {
				limit, parseErr := strconv.Atoi(values[0])
				if parseErr != nil {
					return nil, invalidRequest()
				}
				in[name] = limit
			} else if name != "next" {
				in[name] = values[0]
			}
		}
	}
	if operation == "read" {
		item := map[string]string{"handle": handle}
		if next := query.Get("next"); next != "" {
			item = map[string]string{"next": next}
		}
		in["items"] = []map[string]string{item}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, invalidRequest()
	}
	return raw, nil
}
