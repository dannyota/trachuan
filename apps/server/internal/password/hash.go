package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

const ErrHashInvalid failure = "invalid password hash"
const phcPrefix = "$argon2id$v=19$m=65536,t=3,p=2$"

type Hasher struct {
	admission *Admission
	dummy     string
}

func NewHasher() *Hasher {
	salt := []byte("trachuan-dummy-v")
	key := argon2.IDKey([]byte("synthetic dummy password"), salt, 3, 65536, 2, 32)
	return &Hasher{admission: NewAdmission(), dummy: encode(salt, key)}
}
func (h *Hasher) Hash(ctx context.Context, raw string) (string, error) {
	normalized, err := Normalize(raw)
	if err != nil {
		return "", err
	}
	if err = h.admission.Acquire(ctx); err != nil {
		return "", err
	}
	defer h.admission.Release()
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return "", fmt.Errorf("password salt: %w", err)
	}
	key := argon2.IDKey([]byte(normalized), salt, 3, 65536, 2, 32)
	return encode(salt, key), nil
}
func (h *Hasher) Verify(ctx context.Context, encoded, raw string) (bool, error) {
	if len(encoded) > 192 || !strings.HasPrefix(encoded, phcPrefix) {
		return false, ErrHashInvalid
	}
	fields := strings.Split(strings.TrimPrefix(encoded, phcPrefix), "$")
	if len(fields) != 2 {
		return false, ErrHashInvalid
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(fields[0])
	if err != nil || len(salt) != 16 {
		return false, ErrHashInvalid
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(fields[1])
	if err != nil || len(want) != 32 {
		return false, ErrHashInvalid
	}
	if err = h.admission.Acquire(ctx); err != nil {
		return false, err
	}
	defer h.admission.Release()
	got := argon2.IDKey([]byte(norm.NFC.String(raw)), salt, 3, 65536, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
func (h *Hasher) VerifyDummy(ctx context.Context, raw string) error {
	_, err := h.Verify(ctx, h.dummy, raw)
	return err
}
func encode(salt, key []byte) string {
	return phcPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}
