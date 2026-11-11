package credential

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestIssue_FormatAndHash(t *testing.T) {
	plain, hash, err := Issue(bytes.NewReader(bytes.Repeat([]byte{255}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^trc_[A-Za-z0-9_-]{43}$`).MatchString(plain) {
		t.Fatal("invalid personal token format")
	}
	sum := sha256.Sum256([]byte(plain))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatal("hash must cover the complete token")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("sensitive entropy failure")
}

func TestIssue_EntropyFailureReturnsNoToken(t *testing.T) {
	for _, reader := range []interface{ Read([]byte) (int, error) }{bytes.NewReader(make([]byte, 31)), failingReader{}} {
		plain, hash, err := Issue(reader)
		if err == nil || plain != "" || hash != "" || strings.Contains(err.Error(), "sensitive") {
			t.Fatal("entropy failure must return a safe error and no credentials")
		}
	}
}
