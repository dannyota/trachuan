package webapi

import (
	"encoding/json"
	"net/http"
	"net/url"

	"danny.vn/trachuan/internal/retrieval"
)

func (a *api) documents(r *http.Request) (json.RawMessage, error) {
	key := r.PathValue("key")
	_, anchor, err := retrieval.ParseHandle(key)
	if err != nil || anchor != "" {
		return nil, invalidRequest()
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, invalidRequest()
	}
	for name, values := range query {
		if (name != "release" && name != "next") || len(values) != 1 || values[0] == "" {
			return nil, invalidRequest()
		}
	}
	return a.operations.Document(r.Context(), key, query.Get("release"), query.Get("next"))
}
