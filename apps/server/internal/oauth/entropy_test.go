package oauth

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestEntropy_Secrets(t *testing.T) {
	for _, prefix := range []string{"", "tra_", "trr_"} {
		raw, hash, err := NewSecret(bytes.NewReader(make([]byte, 32)), prefix)
		if err != nil || len(raw) != len(prefix)+43 || !strings.HasPrefix(raw, prefix) || hash != sha256.Sum256([]byte(raw)) {
			t.Fatalf("secret %q %x %v", raw, hash, err)
		}
	}
	if raw, _, err := NewSecret(bytes.NewReader(nil), ""); raw != "" || !errors.Is(err, io.EOF) {
		t.Fatalf("short read: %q %v", raw, err)
	}
}
