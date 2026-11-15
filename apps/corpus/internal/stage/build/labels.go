package build

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

func normalizeLabel(value string) string {
	return norm.NFC.String(strings.Join(strings.Fields(value), " "))
}
