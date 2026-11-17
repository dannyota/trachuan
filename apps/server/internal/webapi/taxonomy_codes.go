package webapi

import (
	"encoding/json"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
)

func NewPublicTaxonomyCodes(cfg mcpapi.Config) http.Handler {
	ops := mcpapi.NewOperations(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			apierror.Write(w, 400, "invalid_request")
			return
		}
		result, err := ops.TaxonomyCodes(r.Context())
		if err != nil {
			apierror.Write(w, 503, "unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=600")
		_ = json.NewEncoder(w).Encode(result)
	})
}
