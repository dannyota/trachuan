package webapi

import (
	"encoding/json"
	"net/http"
)

func (a *api) status(r *http.Request) (json.RawMessage, error) {
	if r.URL.RawQuery != "" {
		return nil, invalidRequest()
	}
	return a.operations.Status(r.Context(), json.RawMessage(`{"country":"vn"}`))
}
