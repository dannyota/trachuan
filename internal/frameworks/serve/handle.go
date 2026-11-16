package serve

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/internal/frameworks/businesskey"
	"danny.vn/trachuan/internal/retrieval"
)

type handle struct{ code, version, citation string }

func parseHandle(raw string) (handle, error) {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > retrieval.ReadHandleMax || strings.ContainsRune(raw, 0) {
		return handle{}, ErrInvalidHandle
	}
	base, encoded, hasCitation := strings.Cut(raw, "#")
	parts := strings.Split(base, ":")
	if len(parts) != 3 || parts[0] != "fw" || !businesskey.New().Valid(parts[1], parts[2], "X") {
		return handle{}, ErrInvalidHandle
	}
	h := handle{code: parts[1], version: parts[2]}
	if hasCitation {
		citation, err := url.PathUnescape(encoded)
		if err != nil || !businesskey.New().Valid(h.code, h.version, citation) || encodeCitation(citation) != encoded {
			return handle{}, ErrInvalidHandle
		}
		h.citation = citation
	}
	return h, nil
}

func (h handle) versionHandle() string { return "fw:" + h.code + ":" + h.version }

func (h handle) String() string {
	value := h.versionHandle()
	if h.citation != "" {
		value += "#" + encodeCitation(h.citation)
	}
	return value
}
