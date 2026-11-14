package seal

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func key(n byte) string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{n}, 32)) }

func TestParse_Keys(t *testing.T) {
	for _, raw := range []string{"", "\n", key(1) + "\n\n", key(1) + "\n" + key(2) + "\n" + key(3), key(1) + "\n" + key(1), " " + key(1), key(1) + "==", strings.Repeat("x", 42), strings.Repeat("!", 43)} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Fatal("accepted malformed ring")
		}
	}
	for _, raw := range []string{key(1), key(1) + "\n", key(1) + "\n" + key(2) + "\n"} {
		if _, err := Parse([]byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSeal_BindingAndRotation(t *testing.T) {
	old, err := Parse([]byte(key(1)))
	if err != nil {
		t.Fatal(err)
	}
	b := Binding{Purpose: "totp", OwnerKind: "account", OwnerID: 12, RowID: 34}
	ciphertext, id, err := old.Seal(b, []byte("synthetic plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := old.Seal(b, []byte("synthetic plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ciphertext, second) {
		t.Fatal("nonce reused")
	}
	ring, err := Parse([]byte(key(2) + "\n" + key(1)))
	if err != nil {
		t.Fatal(err)
	}
	plain, previous, err := ring.Open(b, id, ciphertext)
	if err != nil || !previous || string(plain) != "synthetic plaintext" {
		t.Fatalf("previous=%v err=%v", previous, err)
	}
	sealed, active, err := ring.Seal(b, plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, previous, err := ring.Open(b, active, sealed); err != nil || previous {
		t.Fatalf("active previous=%v err=%v", previous, err)
	}
	for _, changed := range []Binding{{Purpose: "mail", OwnerKind: b.OwnerKind, OwnerID: 12, RowID: 34}, {Purpose: b.Purpose, OwnerKind: "registration", OwnerID: 12, RowID: 34}, {Purpose: b.Purpose, OwnerKind: b.OwnerKind, OwnerID: 13, RowID: 34}, {Purpose: b.Purpose, OwnerKind: b.OwnerKind, OwnerID: 12, RowID: 35}} {
		if _, _, err := ring.Open(changed, id, ciphertext); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("binding accepted: %v", err)
		}
	}
	for _, data := range [][]byte{nil, ciphertext[:10], append(append([]byte{}, ciphertext[:len(ciphertext)-1]...), ciphertext[len(ciphertext)-1]^1)} {
		if _, _, err := ring.Open(b, id, data); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("tamper accepted: %v", err)
		}
	}
	if _, _, err := ring.Open(b, "unknown", ciphertext); !errors.Is(err, ErrUnknownKey) {
		t.Fatal(err)
	}
	if _, _, err := ring.Open(b, active, ciphertext); !errors.Is(err, ErrAuthentication) {
		t.Fatal(err)
	}
}
