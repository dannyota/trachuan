package factor

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestRecovery_NormalizationAndAccountBinding(t *testing.T) {
	code := "ABCDE-FGHIJ-KLMNO-PQRST-UVWXY-Z"
	normalized, ok := NormalizeRecovery(" abcde-fghij klmno-pqrst uvwxy-z ")
	if !ok || normalized != "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		t.Fatal("normalization")
	}
	sum := sha256.Sum256([]byte("trachuan-recovery-v1\x0042\x00ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	if RecoveryDigest(42, normalized) != hex.EncodeToString(sum[:]) || RecoveryDigest(43, normalized) == RecoveryDigest(42, normalized) {
		t.Fatal("digest domain")
	}
	for _, bad := range []string{"", strings.Repeat("A", 25), strings.Repeat("A", 27), "0BCDEFGHIJKLMNOPQRSTUVWXYZ", strings.ReplaceAll(code, "-", "\t")} {
		if _, ok := NormalizeRecovery(bad); ok {
			t.Fatalf("accepted malformed %q", bad)
		}
	}
	codes, err := NewRecoveryCodes()
	if err != nil || len(codes) != 10 {
		t.Fatal("generation", err)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		n, ok := NormalizeRecovery(c)
		if !ok || len(c) != 31 || seen[n] {
			t.Fatal("invalid generated code")
		}
		seen[n] = true
	}
}
