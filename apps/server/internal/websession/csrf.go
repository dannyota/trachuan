package websession

import (
	"crypto/subtle"
	"mime"
	"net/http"
	"net/url"
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
	public, err := url.Parse(m.PublicURL)
	if err != nil || !validOriginURL(public) {
		return false
	}
	allowed := public.Scheme + "://" + public.Host
	if values, present := r.Header[http.CanonicalHeaderKey("Origin")]; present {
		origin, ok := singleHeader(r.Header, "Origin")
		return len(values) == 1 && ok && origin == allowed
	}
	referer, ok := singleHeader(r.Header, "Referer")
	if !ok {
		return false
	}
	u, err := url.Parse(referer)
	return err == nil && validOriginURL(u) && u.Scheme+"://"+u.Host == allowed
}

func validOriginURL(u *url.URL) bool {
	return (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.Hostname() != "" && u.User == nil && u.Opaque == ""
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
