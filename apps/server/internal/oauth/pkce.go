package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// PKCE code-verifier bounds, from RFC 7636 section 4.1.
const (
	pkceVerifierMinChars = 43
	pkceVerifierMaxChars = 128
)

// VerifyS256 reports whether verifier is the PKCE code verifier behind
// challenge under the S256 method (RFC 7636 section 4.6).
//
// It admits only a 43-to-128 character verifier drawn from the RFC 7636
// unreserved alphabet and only the canonical unpadded base64url spelling of a
// 32-byte challenge. The verifier is hashed as ASCII, never decoded; the
// challenge is decoded once and the two digests are compared in constant time.
// Every rejection returns false rather than an error, so no call site can
// branch on the reason.
func VerifyS256(challenge string, verifier string) bool {
	if !isPKCEVerifier(verifier) {
		return false
	}
	if len(challenge) != 43 {
		return false
	}
	want, err := base64.RawURLEncoding.Strict().DecodeString(challenge)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	got := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare(got[:], want) == 1
}

// isPKCEVerifier reports whether raw holds 43 to 128 characters of the RFC
// 7636 verifier alphabet: ALPHA / DIGIT / "-" / "." / "_" / "~". That alphabet
// is ASCII, so byte length is character length and a non-ASCII byte fails the
// alphabet check.
func isPKCEVerifier(raw string) bool {
	if len(raw) < pkceVerifierMinChars || len(raw) > pkceVerifierMaxChars {
		return false
	}
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-', c == '.', c == '_', c == '~':
		default:
			return false
		}
	}
	return true
}

// VerifyPKCE accepts only S256 and verifies the supplied code verifier.
func VerifyPKCE(method, challenge, verifier string) bool {
	return method == "S256" && VerifyS256(challenge, verifier)
}
