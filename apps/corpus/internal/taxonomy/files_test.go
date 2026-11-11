package taxonomy

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFiles_Validation(t *testing.T) {
	m := Mapping{Nodes: []MappedNode{{GUID: "s", Code: "1", Level: 1, Kind: "chude", Ordinal: 1, Status: "active"}, {GUID: "d", Code: "1.1", ParentCode: "1", Level: 2, Kind: "demuc", Ordinal: 1, Status: "active"}}}
	tests := []struct {
		name, custom, overrides string
		valid                   bool
	}{
		{"valid", "- code: 1.1.x1\n  parent: '1.1'\n  name: Name\n  description: A sentence.\n", "- document_key: vn:vbpl:1\n  anchor: dieu-1\n  code: 1.1.x1\n  reason: Reviewed\n", true},
		{"missing parent", "- code: 1.2.x1\n  parent: '1.2'\n  name: Name\n  description: A sentence.\n", "[]", false},
		{"missing description", "- code: 1.1.x1\n  parent: '1.1'\n  name: Name\n", "[]", false},
		{"unknown override", "[]", "- document_key: vn:vbpl:1\n  anchor: dieu-1\n  code: 1.9\n  reason: Reviewed\n", false},
		{"duplicate custom", "- code: 1.1.x1\n  parent: '1.1'\n  name: Name\n  description: A sentence.\n- code: 1.1.x1\n  parent: '1.1'\n  name: Name\n  description: A sentence.\n", "[]", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ReadCustom([]byte(tt.custom))
			if err != nil {
				t.Fatal(err)
			}
			o, err := ReadOverrides([]byte(tt.overrides))
			if err != nil {
				t.Fatal(err)
			}
			err = Validate(m, c, o)
			if (err == nil) != tt.valid {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}
func TestVersion_LengthPrefixed(t *testing.T) {
	h := sha256.New()
	for _, v := range []string{"ab", "c", ""} {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(len(v)))
		h.Write(b[:])
		h.Write([]byte(v))
	}
	want := fmt.Sprintf("%x", h.Sum(nil))
	if got := Version([]byte("ab"), []byte("c"), nil); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestCommittedFiles_Valid(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "taxonomy")
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	mapping, err := ReadMapping(read("mapping.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	custom, err := ReadCustom(read("custom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := ReadOverrides(read("overrides.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err = Validate(mapping, custom, overrides); err != nil {
		t.Fatal(err)
	}
}
