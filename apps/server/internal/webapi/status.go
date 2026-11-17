package webapi

import (
	"context"
	"encoding/json"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
)

func NewPublicStatus(cfg mcpapi.Config, summaries ...func(context.Context, string) (bool, error)) http.Handler {
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
		var body map[string]json.RawMessage
		if err = json.Unmarshal(raw, &body); err != nil {
			apierror.Write(w, 503, "unavailable")
			return
		}
		enabled := false
		if len(summaries) > 0 && summaries[0] != nil {
			var release string
			if err = json.Unmarshal(body["release"], &release); err == nil {
				enabled, err = summaries[0](r.Context(), release)
			}
			if err != nil {
				apierror.Write(w, 503, "unavailable")
				return
			}
		}
		body["features"], _ = json.Marshal(struct {
			PublicSummaries bool `json:"public_summaries"`
		}{enabled})
		raw, err = json.Marshal(body)
		if err != nil {
			apierror.Write(w, 503, "unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
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
