package webapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/frameworks/serve"
)

func newFrameworkRoutes(cfg Config) http.Handler {
	if !cfg.Tools.Frameworks {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			apierror.Write(w, 404, "not_found")
		})
	}
	if cfg.Sessions == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			apierror.Write(w, 503, "unavailable")
		})
	}
	a := &api{cfg: cfg}
	mux := http.NewServeMux()
	for _, route := range []struct {
		pattern, operation, kind string
		class                    limits.Class
	}{
		{"GET /api/v1/frameworks", "corpus_status", "", limits.Read},
		{"GET /api/v1/frameworks/versions/{handle}", "read", "version", limits.Read},
		{"GET /api/v1/frameworks/controls/{handle}", "read", "control", limits.Read},
		{"POST /api/v1/frameworks/search", "search", "", limits.Search},
		{"GET /api/v1/frameworks/mappings", "mappings", "", limits.Read},
		{"GET /api/v1/frameworks/gaps", "quality_gaps", "", limits.Read},
	} {
		operation := mcpapi.NewFrameworkOperation(cfg.Tools, route.operation)
		mux.Handle(route.pattern, a.route("frameworks_"+route.operation, route.class, func(r *http.Request) (json.RawMessage, error) {
			handle := ""
			if route.kind != "" {
				handle = r.PathValue("handle")
				if !serve.IsHandle(handle) || strings.Contains(handle, "#") != (route.kind == "control") {
					return nil, serve.ErrInvalidHandle
				}
			}
			raw, err := frameworkInput(r, route.operation, handle)
			if err != nil {
				return nil, err
			}
			p, _ := websession.FromContext(r.Context())
			return operation(r.Context(), p.AccountID, raw, handle)
		}))
	}
	return mcpapi.NoStore(cfg.Sessions.Middleware(cfg.Sessions.CSRF(mux)))
}
