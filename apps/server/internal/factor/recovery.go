package factor

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

func NormalizeRecovery(raw string) (string, bool) {
	code := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(raw))
	if len(code) != 26 {
		return "", false
	}
	for _, c := range code {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '7') {
			return "", false
		}
	}
	return code, true
}
func RecoveryDigest(accountID int64, code string) string {
	sum := sha256.Sum256([]byte("trachuan-recovery-v1\x00" + strconv.FormatInt(accountID, 10) + "\x00" + code))
	return hex.EncodeToString(sum[:])
}
func NewRecoveryCodes() ([]string, error) {
	codes := make([]string, 10)
	for i := range codes {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, fmt.Errorf("generate recovery code: %w", err)
		}
		code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])
		codes[i] = code[:5] + "-" + code[5:10] + "-" + code[10:15] + "-" + code[15:20] + "-" + code[20:25] + "-" + code[25:]
	}
	return codes, nil
}
