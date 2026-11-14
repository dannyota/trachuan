package app

import "testing"

func TestConfig_TOTPEnrollment(t *testing.T) {
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	for _, tc := range []struct {
		raw            string
		enabled, valid bool
	}{{"", false, true}, {"false", false, true}, {"true", true, true}, {"typo", false, false}} {
		t.Setenv("SERVER_TOTP_ENROLLMENT", tc.raw)
		c, err := loadConfig()
		if (err == nil) != tc.valid || err == nil && c.totpEnrollment != tc.enabled {
			t.Fatalf("value %q: enabled=%v error=%v", tc.raw, c.totpEnrollment, err)
		}
	}
}
