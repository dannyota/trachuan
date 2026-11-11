package webapi

import (
	"encoding/json"
	"net/http"
)

func (a *api) read(r *http.Request) (json.RawMessage, error) {
	raw, err := requestBody(r)
	if err != nil {
		return nil, err
	}
	return a.operations.Read(r.Context(), raw)
}
