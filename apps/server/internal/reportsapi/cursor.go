package reportsapi

import (
	"encoding/base64"
	"encoding/binary"
	"time"
)

func encodeCursor(t time.Time, id int64) string {
	var raw [17]byte
	raw[0] = 1
	binary.BigEndian.PutUint64(raw[1:9], uint64(t.UnixMicro()))
	binary.BigEndian.PutUint64(raw[9:], uint64(id))
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
func decodeCursor(value string) (time.Time, int64, bool) {
	if len(value) != base64.RawURLEncoding.EncodedLen(17) {
		return time.Time{}, 0, false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(raw) != 17 || raw[0] != 1 {
		return time.Time{}, 0, false
	}
	t := time.UnixMicro(int64(binary.BigEndian.Uint64(raw[1:9]))).UTC()
	id := int64(binary.BigEndian.Uint64(raw[9:]))
	if id <= 0 || t.Year() < 1 || t.Year() > 9999 || encodeCursor(t, id) != value {
		return time.Time{}, 0, false
	}
	return t, id, true
}
