package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// NewSecret reads 32 bytes and returns a prefixed secret and its SHA-256 digest.
func NewSecret(entropy io.Reader, prefix string) (string, [32]byte, error) {
	var buf [32]byte
	if _, err := io.ReadFull(entropy, buf[:]); err != nil {
		return "", [32]byte{}, fmt.Errorf("read oauth entropy: %w", err)
	}
	raw := prefix + base64.RawURLEncoding.EncodeToString(buf[:])
	return raw, sha256.Sum256([]byte(raw)), nil
}
