package fetch

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"strings"
)

// The divisor is set in docs/design/budgets.md.
const controlDivisor = 20

func controlSample(key, rel string) bool {
	h := sha256.New()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(rel))
	return binary.BigEndian.Uint64(h.Sum(nil)[:8])%controlDivisor == 0
}

func compareControl(b *baseline, tree, files, report []byte) (string, []string) {
	var differs []string
	if !bytes.Equal(b.tree, tree) {
		differs = append(differs, "tree.json")
	}
	if !bytes.Equal(b.filesJSON, files) {
		differs = append(differs, "files.json")
	}
	if !bytes.Equal(b.reportJSON, report) {
		differs = append(differs, "report.json")
	}
	if len(differs) == 0 {
		return "same", differs
	}
	if len(differs) != 1 || differs[0] != "tree.json" {
		return "changed", differs
	}
	const responseLimit = 20_000_000
	if len(b.tree) > responseLimit || len(tree) > responseLimit || !json.Valid(b.tree) || !json.Valid(tree) {
		return "changed", differs
	}
	var old, fresh any
	oldDecoder := json.NewDecoder(bytes.NewReader(b.tree))
	freshDecoder := json.NewDecoder(bytes.NewReader(tree))
	oldDecoder.UseNumber()
	freshDecoder.UseNumber()
	if oldDecoder.Decode(&old) != nil || freshDecoder.Decode(&fresh) != nil {
		return "changed", differs
	}
	stripIDs(old)
	stripIDs(fresh)
	if reflect.DeepEqual(old, fresh) {
		return "tree_ids_only", differs
	}
	return "changed", differs
}

func stripIDs(value any) {
	switch v := value.(type) {
	case map[string]any:
		delete(v, "key")
		delete(v, "id")
		for _, child := range v {
			stripIDs(child)
		}
	case []any:
		for _, child := range v {
			stripIDs(child)
		}
	}
}

func (d readDecision) message() string {
	result := "read=" + d.reason
	if d.outcome != "" {
		result += " outcome=" + d.outcome
	}
	return result
}

func parseReadMessage(message string) readDecision {
	var d readDecision
	for _, part := range strings.Fields(message) {
		if strings.HasPrefix(part, "read=") {
			d.reason = strings.TrimPrefix(part, "read=")
		}
		if strings.HasPrefix(part, "outcome=") {
			d.outcome = strings.TrimPrefix(part, "outcome=")
		}
	}
	return d
}
