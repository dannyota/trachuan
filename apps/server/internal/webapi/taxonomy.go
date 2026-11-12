package webapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
)

// NewPublicTaxonomy serves taxonomy without credentials.
func NewPublicTaxonomy(cfg mcpapi.Config) http.Handler {
	ops := mcpapi.NewOperations(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			apierror.Write(w, 400, "invalid_request")
			return
		}
		code := r.PathValue("code")
		input := map[string]any{"country": "vn"}
		for key, values := range query {
			if code != "" || (key != "node" && key != "depth") || len(values) != 1 || values[0] == "" {
				apierror.Write(w, 400, "invalid_request")
				return
			}
			if key == "depth" {
				depth, parseErr := strconv.Atoi(values[0])
				if parseErr != nil || depth < 1 || depth > 3 {
					apierror.Write(w, 400, "invalid_request")
					return
				}
				input[key] = depth
			} else {
				input[key] = values[0]
			}
		}
		var raw json.RawMessage
		if code != "" {
			raw, err = ops.TaxonomyNode(r.Context(), code)
		} else {
			data, _ := json.Marshal(input)
			raw, err = ops.Taxonomy(r.Context(), data)
		}
		if err != nil {
			status, c := httpError(err)
			apierror.Write(w, status, c)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=600")
		_, _ = w.Write(raw)
	})
}
