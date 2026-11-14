package seal

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

func TestBinding_RejectsAmbiguousOrEmptyComponents(t *testing.T) {
	ring, err := Parse([]byte(key(1)))
	if err != nil {
		t.Fatal(err)
	}
	_, id, err := ring.Seal(Binding{Purpose: "totp", OwnerKind: "account", OwnerID: 12, RowID: 34}, []byte("synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		binding Binding
		aad     string
	}{
		{"collision purpose", Binding{Purpose: "totp|account", OwnerKind: "registration", OwnerID: 12, RowID: 34}, "totp|account|registration|12|34|"},
		{"collision owner", Binding{Purpose: "totp", OwnerKind: "account|registration", OwnerID: 12, RowID: 34}, "totp|account|registration|12|34|"},
		{"empty purpose", Binding{Purpose: "", OwnerKind: "account", OwnerID: 12, RowID: 34}, "|account|12|34|"},
		{"empty owner", Binding{Purpose: "totp", OwnerKind: "", OwnerID: 12, RowID: 34}, "totp||12|34|"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("Seal", func(t *testing.T) {
				sealed, keyID, err := ring.Seal(tc.binding, []byte("synthetic"))
				if err == nil || len(sealed) != 0 || keyID != "" {
					t.Fatal("invalid binding sealed")
				}
			})
			t.Run("Open", func(t *testing.T) {
				nonce := make([]byte, 12)
				legacy := gcm.Seal(nonce, nonce, []byte("synthetic"), []byte(tc.aad+id))
				plain, previous, err := ring.Open(tc.binding, id, legacy)
				if err == nil || len(plain) != 0 || previous {
					t.Fatal("invalid binding opened")
				}
			})
		})
	}
}
