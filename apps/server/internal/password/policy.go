package password

import (
	"embed"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const ErrRejected failure = "password rejected"

//go:embed blocklist/passwords.txt blocklist/LICENSE
var bundled embed.FS

func commonPasswords() string {
	content, _ := bundled.ReadFile("blocklist/passwords.txt")
	return string(content)
}

func Normalize(raw string) (string, error) {
	if !utf8.ValidString(raw) || len(raw) > 1024 {
		return "", ErrRejected
	}
	normalized := norm.NFC.String(raw)
	if n := utf8.RuneCountInString(normalized); n < 15 || n > 128 || len(normalized) > 1024 {
		return "", ErrRejected
	}
	return normalized, nil
}

func Check(raw, address string) (string, error) {
	normalized, err := Normalize(raw)
	if err != nil {
		return "", err
	}
	address = norm.NFC.String(address)
	local, _, _ := strings.Cut(address, "@")
	if normalized == address || normalized == local {
		return "", ErrRejected
	}
	for entry := range strings.SplitSeq(commonPasswords(), "\n") {
		if normalized == entry {
			return "", ErrRejected
		}
	}
	return normalized, nil
}
