package store

import "errors"

func ParseFrameworkScope(raw string) (string, error) {
	if raw == "*" {
		return raw, nil
	}
	if len(raw) == 0 || raw[0] < 'a' || raw[0] > 'z' || raw[len(raw)-1] == '-' {
		return "", errors.New("invalid framework scope")
	}
	for i, c := range []byte(raw) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		if c == '-' && i > 0 && raw[i-1] != '-' {
			continue
		}
		return "", errors.New("invalid framework scope")
	}
	return raw, nil
}
