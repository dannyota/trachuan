package retrieval

import (
	"errors"
	"strings"
)

// Handle returns the public handle for a document or provision identity.
func Handle(id Identity) string {
	if id.Anchor == "" {
		return id.DocumentKey
	}
	return id.DocumentKey + "#" + id.Anchor
}

// ParseHandle splits a public handle into its document key and optional anchor.
func ParseHandle(s string) (key, anchor string, err error) {
	key, anchor, hasAnchor := strings.Cut(s, "#")
	if !strings.HasPrefix(key, "vn:") || (hasAnchor && anchor == "") {
		return "", "", errors.New("invalid handle")
	}
	return key, anchor, nil
}
