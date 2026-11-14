package accountemail

import (
	"errors"
	"strings"
)

func Parse(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	local, domain, found := strings.Cut(address, "@")
	if !found || len(address) > 254 || len(local) < 1 || len(local) > 64 || strings.Contains(domain, "@") || !strings.Contains(domain, ".") {
		return "", errors.New("invalid email address")
	}
	for _, atom := range strings.Split(local, ".") {
		if atom == "" {
			return "", errors.New("invalid email address")
		}
		for _, c := range []byte(atom) {
			if !asciiLetter(c) && (c < '0' || c > '9') && !strings.ContainsRune("!#$%&'*+-/=?^_`{|}~", rune(c)) {
				return "", errors.New("invalid email address")
			}
		}
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("invalid email address")
		}
		for _, c := range []byte(label) {
			if !asciiLetter(c) && (c < '0' || c > '9') && c != '-' {
				return "", errors.New("invalid email address")
			}
		}
	}
	return local + "@" + strings.ToLower(domain), nil
}

func asciiLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
