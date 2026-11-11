package oauth

// ParseScope validates the single supported scope, defaulting an empty value.
func ParseScope(raw string) (string, error) {
	if raw == "" || raw == "mcp:read" {
		return "mcp:read", nil
	}
	return "", ErrScopeInvalid
}
