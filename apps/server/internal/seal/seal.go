package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

type failure string

func (e failure) Error() string { return string(e) }

const (
	ErrUnknownKey     failure = "unknown sealing key"
	ErrAuthentication failure = "sealed value authentication failed"
)

type Binding struct {
	Purpose, OwnerKind string
	OwnerID, RowID     int64
}

type ringKey struct {
	id   string
	aead cipher.AEAD
}
type Ring struct{ keys []ringKey }

func Parse(content []byte) (*Ring, error) {
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n"), "\n")
	if len(lines) < 1 || len(lines) > 2 {
		return nil, fmt.Errorf("invalid sealing key ring")
	}
	ring := &Ring{}
	for _, line := range lines {
		encoding := base64.RawURLEncoding.Strict()
		if len(line) == 44 && strings.HasSuffix(line, "=") {
			encoding = base64.URLEncoding.Strict()
		}
		raw, err := encoding.DecodeString(line)
		if err != nil || len(raw) != 32 || encoding.EncodeToString(raw) != line {
			return nil, fmt.Errorf("invalid sealing key ring")
		}
		hash := sha256.Sum256(raw)
		id := hex.EncodeToString(hash[:8])
		if len(ring.keys) > 0 && ring.keys[0].id == id {
			return nil, fmt.Errorf("duplicate sealing key")
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, fmt.Errorf("create sealing cipher: %w", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("create sealing mode: %w", err)
		}
		ring.keys = append(ring.keys, ringKey{id: id, aead: aead})
	}
	return ring, nil
}

func (r *Ring) Seal(binding Binding, plaintext []byte) ([]byte, string, error) {
	if r == nil || len(r.keys) == 0 {
		return nil, "", ErrUnknownKey
	}
	k := r.keys[0]
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, "", fmt.Errorf("generate sealing nonce: %w", err)
	}
	return k.aead.Seal(nonce, nonce, plaintext, binding.data(k.id)), k.id, nil
}

func (r *Ring) Open(binding Binding, id string, sealed []byte) ([]byte, bool, error) {
	if r != nil {
		for i, k := range r.keys {
			if k.id != id {
				continue
			}
			if len(sealed) < k.aead.NonceSize()+k.aead.Overhead() {
				return nil, false, ErrAuthentication
			}
			plaintext, err := k.aead.Open(nil, sealed[:k.aead.NonceSize()], sealed[k.aead.NonceSize():], binding.data(id))
			if err != nil {
				return nil, false, ErrAuthentication
			}
			return plaintext, i > 0, nil
		}
	}
	return nil, false, ErrUnknownKey
}

func (b Binding) data(id string) []byte {
	return []byte(fmt.Sprintf("%s|%s|%d|%d|%s", b.Purpose, b.OwnerKind, b.OwnerID, b.RowID, id))
}
