package extract

import (
	"html"
	"regexp"

	"golang.org/x/text/unicode/norm"
)

func textFromHTML(source string) string {
	breaks := regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	tags := regexp.MustCompile(`<[^>]*>`)
	source = breaks.ReplaceAllString(source, "\n")
	source = tags.ReplaceAllString(source, "")
	return norm.NFC.String(html.UnescapeString(source))
}
