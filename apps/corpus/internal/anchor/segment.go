// Package anchor composes public provision anchor segments.
package anchor

import "strings"

// Segment joins a provision kind and its lowercased label token.
func Segment(kind, token string) string { return kind + "-" + strings.ToLower(token) }
