package oauth

import "testing"

func TestPKCE_S256Only(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if !VerifyPKCE("S256", challenge, verifier) || VerifyPKCE("plain", verifier, verifier) || VerifyPKCE("S256", challenge, verifier+"!") {
		t.Fatal("PKCE verification failed")
	}
}
