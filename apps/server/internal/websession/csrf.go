package websession

import (
	"crypto/subtle"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// CSRF checks origin, session secret and JSON media type on unsafe web requests.
// Install it inside Middleware so the session principal is present.
func (m *Manager) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || r.URL.Path != "/api/v1" && !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		p, ok := FromContext(r.Context())
		token, tokenOK := singleHeader(r.Header, "X-CSRF-Token")
		if !ok || !tokenOK || !validValue(p.CSRFSecret) || subtle.ConstantTimeCompare([]byte(token), []byte(p.CSRFSecret)) != 1 || !m.originAllowed(r) || hasBody(r) && !jsonBody(r.Header) {
			writeError(w, 403, "csrf_rejected", "CSRF validation failed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) originAllowed(r *http.Request) bool {
	allowed, ok := parseOrigin(m.PublicURL, false)
	if !ok {
		return false
	}
	if values, present := r.Header[http.CanonicalHeaderKey("Origin")]; present {
		raw, present := singleHeader(r.Header, "Origin")
		origin, valid := parseOrigin(raw, true)
		return len(values) == 1 && present && valid && origin == allowed
	}
	referer, ok := singleHeader(r.Header, "Referer")
	if !ok {
		return false
	}
	origin, valid := parseOrigin(referer, false)
	return valid && origin == allowed
}

type origin struct {
	scheme, host, port string
}

func parseOrigin(raw string, onlyOrigin bool) (origin, bool) {
	u, err := url.Parse(raw)
	if err != nil || strings.Contains(raw, "#") || u.User != nil || u.Opaque != "" || u.Host == "" || u.Hostname() == "" || strings.HasSuffix(u.Host, ":") {
		return origin{}, false
	}
	scheme, host := strings.ToLower(u.Scheme), strings.ToLower(u.Hostname())
	if (scheme != "https" && scheme != "http") || (onlyOrigin && (u.Path != "" || u.RawQuery != "" || u.ForceQuery)) {
		return origin{}, false
	}
	if strings.HasPrefix(u.Host, "[") && net.ParseIP(host) == nil {
		return origin{}, false
	}
	port := u.Port()
	if port != "" {
		n, portErr := strconv.ParseUint(port, 10, 16)
		if portErr != nil {
			return origin{}, false
		}
		port = strconv.FormatUint(n, 10)
		if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
			port = ""
		}
	}
	return origin{scheme: scheme, host: host, port: port}, true
}

func singleHeader(h http.Header, name string) (string, bool) {
	values := h.Values(name)
	if len(values) != 1 || values[0] == "" || strings.Contains(values[0], ",") {
		return "", false
	}
	return values[0], true
}

func hasBody(r *http.Request) bool { return r.ContentLength != 0 || len(r.TransferEncoding) > 0 }

func jsonBody(h http.Header) bool {
	values := h.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(values[0])
	return err == nil && mediaType == "application/json"
}
