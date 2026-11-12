package vbpl

import (
	"regexp"
	"strings"
)

// ValidDocumentID reports whether id is an explicit VBPL target document ID.
func ValidDocumentID(id string) bool {
	valid, _ := regexp.MatchString(`^[0-9A-Za-z][0-9A-Za-z_-]{0,63}$`, id)
	return valid && !strings.HasPrefix(strings.ToLower(id), "vbpqta_")
}
