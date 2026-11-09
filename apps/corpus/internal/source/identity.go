package source

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CentralListingIdentity hashes the unfiltered central listing definition.
func CentralListingIdentity(source, level string, pageSize int) string {
	return queryIdentity(map[string]any{"source": source, "type": "listing", "level": level, "page_size": pageSize})
}

// NamedLookupIdentity hashes a named number lookup definition.
func NamedLookupIdentity(source, option, mode, level, number string) string {
	return queryIdentity(map[string]any{"source": source, "type": "named", "option": option, "mode": mode, "level": level, "number": number})
}

func queryIdentity(definition map[string]any) string {
	data, _ := json.Marshal(definition)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
