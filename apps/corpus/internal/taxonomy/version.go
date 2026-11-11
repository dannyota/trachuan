package taxonomy

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Version hashes the length-prefixed contents of the taxonomy files.
func Version(mapping, custom, overrides []byte) string {
	h := sha256.New()
	for _, b := range [][]byte{mapping, custom, overrides} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(b)))
		_, _ = h.Write(size[:])
		_, _ = h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}
