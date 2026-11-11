package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestPKCE_S256Only(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if !VerifyPKCE("S256", challenge, verifier) || VerifyPKCE("plain", verifier, verifier) || VerifyPKCE("S256", challenge, verifier+"!") {
		t.Fatal("PKCE verification failed")
	}
}

func TestPKCE_VerifierBoundsAndAlphabet(t *testing.T) {
	challenge := func(verifier string) string {
		digest := sha256.Sum256([]byte(verifier))
		return base64.RawURLEncoding.EncodeToString(digest[:])
	}
	for _, tc := range []struct {
		length int
		want   bool
	}{{42, false}, {43, true}, {128, true}, {129, false}} {
		verifier := strings.Repeat("a", tc.length)
		if got := VerifyPKCE("S256", challenge(verifier), verifier); got != tc.want {
			t.Errorf("verifier length %d: got %v, want %v", tc.length, got, tc.want)
		}
	}
	for c := 0; c < 256; c++ {
		b := byte(c)
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("-._~", rune(b)) {
			continue
		}
		verifier := strings.Repeat("a", 42) + string([]byte{b})
		if VerifyPKCE("S256", challenge(verifier), verifier) {
			t.Errorf("accepted verifier byte %#x", b)
		}
	}
}

func TestPKCE_ChallengeEncoding(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	for _, tc := range []struct{ name, challenge string }{
		{"short", challenge[:42]}, {"long", challenge + "A"}, {"padding", challenge[:42] + "="},
		{"plus", "+" + challenge[1:]}, {"slash", "/" + challenge[1:]}, {"newline", challenge[:20] + "\n" + challenge[21:]},
		{"noncanonical trailing bits", challenge[:42] + "N"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if VerifyPKCE("S256", tc.challenge, verifier) {
				t.Fatal("accepted malformed challenge")
			}
		})
	}
}
