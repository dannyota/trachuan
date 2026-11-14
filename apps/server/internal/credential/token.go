package credential

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	"danny.vn/trachuan/apps/server/internal/store"
)

type failure string

func (e failure) Error() string { return string(e) }

const (
	errEntropy    failure = "random source unavailable"
	ErrTokenLimit         = store.ErrTokenLimit
)

func Issue(rand io.Reader) (plain, hash string, err error) {
	var random [32]byte
	if rand == nil {
		return "", "", fmt.Errorf("issue personal token: %w", errEntropy)
	}
	if _, err := io.ReadFull(rand, random[:]); err != nil {
		return "", "", fmt.Errorf("issue personal token: %w", errEntropy)
	}
	plain = "trc_" + base64.RawURLEncoding.EncodeToString(random[:])
	return plain, tokenHash(plain), nil
}

func tokenHash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func validToken(plain string) bool {
	if len(plain) != 47 || (plain[:4] != "trc_" && plain[:4] != "tra_") {
		return false
	}
	for _, c := range []byte(plain[4:]) {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
