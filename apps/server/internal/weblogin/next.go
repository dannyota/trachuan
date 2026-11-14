package weblogin

import (
	"net/url"
	"strings"
)

func ValidateNext(raw string) string {
	if len(raw) > 512 || !safeNext(raw) {
		return "/"
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || !safeNext(decoded) {
		return "/"
	}
	return raw
}

func safeNext(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, `\`) {
		return false
	}
	for _, c := range value {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	path, _, _ := strings.Cut(value, "?")
	path, _, _ = strings.Cut(path, "#")
	for segment := range strings.SplitSeq(path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}
