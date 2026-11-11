package oauth

import (
	"io"
	"mime"
	"net/http"
	"net/url"
)

func parseTokenForm(r *http.Request, resource string) (url.Values, string) {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/x-www-form-urlencoded" || len(r.Header.Values("Content-Type")) != 1 {
		return nil, "invalid_request"
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, "invalid_request"
	}
	form, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, "invalid_request"
	}
	for _, values := range form {
		if len(values) != 1 {
			return nil, "invalid_request"
		}
	}
	for _, key := range []string{"grant_type", "client_id", "resource"} {
		if form.Get(key) == "" {
			return nil, "invalid_request"
		}
	}
	switch form.Get("grant_type") {
	case "authorization_code":
		if form.Get("code") == "" || form.Get("code_verifier") == "" {
			return nil, "invalid_request"
		}
	case "refresh_token":
		if form.Get("refresh_token") == "" {
			return nil, "invalid_request"
		}
		if scope, ok := form["scope"]; ok && scope[0] != "mcp:read" {
			return nil, "invalid_scope"
		}
	default:
		return nil, "unsupported_grant_type"
	}
	if form.Get("resource") != resource {
		return nil, "invalid_target"
	}
	return form, ""
}
