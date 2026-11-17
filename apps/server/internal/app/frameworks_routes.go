package app

import (
	"net/http"
	"strings"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
)

type webMux struct {
	*http.ServeMux
	frameworks http.Handler
}

func (m *webMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if frameworkPath(r.URL.EscapedPath()) || frameworkPath(r.URL.Path) {
		m.frameworks.ServeHTTP(w, r)
		return
	}
	m.ServeMux.ServeHTTP(w, r)
}

func frameworkPath(path string) bool {
	return path == "/api/v1/frameworks" || strings.HasPrefix(path, "/api/v1/frameworks/")
}

func guardFrameworkRoutes(enabled bool, next http.Handler) http.Handler {
	return mcpapi.WithCacheControl("no-store", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			apierror.Write(w, 404, "not_found")
			return
		}
		if code := frameworkPathError(r); code != "" {
			apierror.Write(w, 400, code)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func frameworkPathError(r *http.Request) string {
	malformed := strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%2f")
	for _, segment := range strings.Split(r.URL.Path, "/")[1:] {
		if segment == "" || segment == "." || segment == ".." {
			malformed = true
			break
		}
	}
	if !malformed {
		return ""
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/frameworks/controls/") || strings.HasPrefix(r.URL.Path, "/api/v1/frameworks/versions/") {
		return "invalid_handle"
	}
	return "invalid_request"
}
