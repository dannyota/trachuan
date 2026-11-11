package webapi

import (
	"encoding/json"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
)

// NewPublicStatus serves corpus status without using credentials.
func NewPublicStatus(cfg mcpapi.Config) http.Handler {
	a := &api{operations: mcpapi.NewOperations(cfg)}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := a.status(r)
		if err != nil {
			if r.URL.RawQuery != "" {
				apierror.Write(w, http.StatusBadRequest, "invalid_request")
			} else {
				apierror.Write(w, http.StatusServiceUnavailable, "unavailable")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	})
}

func (a *api) status(r *http.Request) (json.RawMessage, error) {
	if r.URL.RawQuery != "" {
		return nil, invalidRequest()
	}
	return a.operations.Status(r.Context(), json.RawMessage(`{"country":"vn"}`))
}
