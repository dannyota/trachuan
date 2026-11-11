package oauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func parseCIMD(raw []byte, id string) (Client, error) {
	fields, err := metadataObject(raw)
	if err != nil {
		return Client{}, err
	}
	var documentID string
	if !metadataString(fields, "client_id", &documentID) || documentID != id {
		return Client{}, ErrInvalidClient
	}
	c, code := clientMetadata(fields, true)
	if code != "" {
		return Client{}, ErrInvalidClient
	}
	c.ID = id
	c.Kind = "cimd"
	return c, nil
}

// metadataObject rejects duplicate members, including ignored nested values.
func metadataObject(raw []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return nil, ErrInvalidClient
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := uniqueJSON(d, 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrInvalidClient
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode client metadata: %w", err)
	}
	return fields, nil
}

func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrInvalidClient
	}
	token, err := d.Token()
	if err != nil {
		return ErrInvalidClient
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return ErrInvalidClient
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, keyErr := d.Token()
			if keyErr != nil {
				return ErrInvalidClient
			}
			name, valid := key.(string)
			if !valid || seen[name] {
				return ErrInvalidClient
			}
			seen[name] = true
		}
		if nestedErr := uniqueJSON(d, depth+1); nestedErr != nil {
			return nestedErr
		}
	}
	end, err := d.Token()
	if err != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return ErrInvalidClient
	}
	return nil
}

func metadataString(fields map[string]json.RawMessage, key string, out *string) bool {
	raw, ok := fields[key]
	return ok && string(bytes.TrimSpace(raw)) != "null" && json.Unmarshal(raw, out) == nil
}

func metadataList(fields map[string]json.RawMessage, key string) ([]string, bool) {
	raw, ok := fields[key]
	if !ok {
		return nil, false
	}
	var members []json.RawMessage
	if json.Unmarshal(raw, &members) != nil || members == nil {
		return nil, false
	}
	values := make([]string, 0, len(members))
	for _, member := range members {
		var value string
		if string(bytes.TrimSpace(member)) == "null" || json.Unmarshal(member, &value) != nil {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func clientMetadata(fields map[string]json.RawMessage, cimd bool) (Client, string) {
	bad := func(code string) (Client, string) { return Client{}, code }
	for _, key := range []string{"client_secret", "client_secret_expires_at"} {
		if _, ok := fields[key]; ok {
			return bad("invalid_client_metadata")
		}
	}
	var name string
	if _, ok := fields["client_name"]; ok {
		if !metadataString(fields, "client_name", &name) {
			return bad("invalid_client_metadata")
		}
		name = norm.NFC.String(strings.TrimSpace(name))
		if len(name) > 256 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return bad("invalid_client_metadata")
		}
	}
	var method string
	if _, ok := fields["token_endpoint_auth_method"]; ok && (!metadataString(fields, "token_endpoint_auth_method", &method) || method != "none") {
		return bad("invalid_client_metadata")
	}
	if _, ok := fields["token_endpoint_auth_methods_supported"]; ok {
		methods, valid := metadataList(fields, "token_endpoint_auth_methods_supported")
		if !valid || len(methods) == 0 {
			return bad("invalid_client_metadata")
		}
		for _, m := range methods {
			if m != "none" {
				return bad("invalid_client_metadata")
			}
		}
	}
	redirects, ok := metadataList(fields, "redirect_uris")
	if !ok || len(redirects) < 1 || len(redirects) > 5 {
		return bad("invalid_redirect_uri")
	}
	for _, raw := range redirects {
		if !validClientRedirect(raw) {
			return bad("invalid_redirect_uri")
		}
	}
	for _, key := range []string{"grant_types", "response_types"} {
		if _, exists := fields[key]; !exists {
			continue
		}
		values, valid := metadataList(fields, key)
		if !valid || len(values) == 0 {
			return bad("invalid_client_metadata")
		}
		if cimd {
			required := "authorization_code"
			if key == "response_types" {
				required = "code"
			}
			if !slices.Contains(values, required) {
				return bad("invalid_client_metadata")
			}
			continue
		}
		seen := map[string]bool{}
		for _, v := range values {
			if seen[v] || (key == "grant_types" && v != "authorization_code" && v != "refresh_token") || (key == "response_types" && v != "code") {
				return bad("invalid_client_metadata")
			}
			seen[v] = true
		}
	}
	var scope string
	if _, ok := fields["scope"]; ok && (!metadataString(fields, "scope", &scope) || scope != "mcp:read") {
		return bad("invalid_client_metadata")
	}
	return Client{Name: name, RedirectURIs: redirects}, ""
}

func validClientRedirect(raw string) bool {
	u, ok := redirectURI(raw)
	if !ok || strings.Contains(raw, `\`) || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if u.Port() != "" {
		port, err := strconv.ParseUint(u.Port(), 10, 16)
		if err != nil || port == 0 {
			return false
		}
	}
	// url.Parse accepts a bracketed DNS label as a host; redirects must have
	// a usable authority before any browser is sent there.
	if strings.HasPrefix(u.Host, "[") && !strings.Contains(u.Hostname(), ":") {
		return false
	}
	return true
}

func cimdURL(raw string) (*url.URL, error) {
	if len(raw) > 2048 || !strings.HasPrefix(raw, "https://") || strings.ContainsAny(raw, "#\\") {
		return nil, ErrInvalidClient
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Hostname() == "" {
		return nil, ErrInvalidClient
	}
	if strings.Trim(u.Path, "/") == "" {
		return nil, ErrInvalidClient
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, ErrInvalidClient
		}
	}
	for _, r := range raw {
		if r < 0x21 || r > 0x7e {
			return nil, ErrInvalidClient
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, ErrInvalidClient
	}
	if u.Port() != "" {
		port, err := strconv.ParseUint(u.Port(), 10, 16)
		if err != nil || port == 0 {
			return nil, ErrInvalidClient
		}
	}
	host := u.Hostname()
	if len(host) > 253 || !strings.Contains(host, ".") {
		return nil, ErrInvalidClient
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return nil, ErrInvalidClient
		}
		for _, r := range label {
			if r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				return nil, ErrInvalidClient
			}
		}
	}
	return u, nil
}
