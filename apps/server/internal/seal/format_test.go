package seal

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"testing"
)

func TestParse_Base64URLLines(t *testing.T) {
	padded := base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	for _, raw := range []string{padded, padded + "\n" + key(2), key(1) + "\r\n" + key(2) + "\r\n"} {
		if _, err := Parse([]byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSeal_StandardGCMEnvelopeAndKeyID(t *testing.T) {
	ring, err := Parse([]byte(base64.RawURLEncoding.EncodeToString(make([]byte, 32))))
	if err != nil {
		t.Fatal(err)
	}
	b := Binding{Purpose: "totp", OwnerKind: "account", OwnerID: 12, RowID: 34}
	sealed, id, err := ring.Seal(b, []byte("synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	if id != "66687aadf862bd77" {
		t.Fatal("incorrect key identifier")
	}
	block, err := aes.NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) != 12+len("synthetic")+gcm.Overhead() {
		t.Fatal("incorrect nonce or tag size")
	}
	plain, err := gcm.Open(nil, sealed[:12], sealed[12:], []byte("totp|account|12|34|66687aadf862bd77"))
	if err != nil || string(plain) != "synthetic" {
		t.Fatal("envelope or associated data differs from contract")
	}
}
