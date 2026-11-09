package evaluation

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed testdata/vn/*.json testdata/vn/inventory.sha256
var vnFixtures embed.FS

// VNSuite loads the embedded Vietnamese evaluation suite.
func VNSuite() (Suite, error) { return LoadSuiteFS(vnFixtures, "testdata/vn") }

// LoadSuiteFS reads and validates an evaluation suite from a filesystem.
func LoadSuiteFS(fsys fs.FS, dir string) (Suite, error) {
	read := func(name string) ([]byte, error) {
		b, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		return b, nil
	}
	legacy, err := read("legacy.json")
	if err != nil {
		return Suite{}, err
	}
	cases, err := LoadLegacy(bytes.NewReader(legacy))
	if err != nil {
		return Suite{}, err
	}
	product, err := read("old-product.json")
	if err != nil {
		return Suite{}, err
	}
	mapping, err := read("identity-map.json")
	if err != nil {
		return Suite{}, err
	}
	fixture, err := read("fixtures.json")
	if err != nil {
		return Suite{}, err
	}
	inventory, err := read("inventory.sha256")
	if err != nil {
		return Suite{}, err
	}
	hash := func(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
	var meta struct {
		SHA256           string `json:"sha256"`
		CaseCount        int    `json:"case_count"`
		SourcePath       string `json:"source_path"`
		SourceRepository string `json:"source_repository"`
		SourceCommit     string `json:"source_commit"`
		Origin           string `json:"origin"`
	}
	if err = json.Unmarshal(fixture, &meta); err != nil {
		return Suite{}, fmt.Errorf("decode fixture metadata: %w", err)
	}
	if meta.SHA256 != hash(legacy) || meta.CaseCount != len(cases) || meta.SourcePath == "" || meta.SourceRepository == "" || meta.SourceCommit == "" || meta.Origin == "" {
		return Suite{}, fmt.Errorf("invalid fixture metadata")
	}
	var old struct {
		LegacySHA256 string   `json:"legacy_sha256"`
		Rejections   []string `json:"rejection_ids"`
	}
	if err = json.Unmarshal(product, &old); err != nil {
		return Suite{}, fmt.Errorf("decode old product: %w", err)
	}
	if old.LegacySHA256 != hash(legacy) {
		return Suite{}, fmt.Errorf("old product legacy hash mismatch")
	}
	m, err := LoadIdentityMap(bytes.NewReader(mapping))
	if err != nil {
		return Suite{}, err
	}
	inv := strings.TrimSpace(string(inventory))
	if len(inv) != 64 {
		return Suite{}, fmt.Errorf("invalid inventory hash")
	}
	if _, err = hex.DecodeString(inv); err != nil {
		return Suite{}, fmt.Errorf("invalid inventory hash: %w", err)
	}
	s := Suite{Legacy: cases, Map: m, Rejections: old.Rejections, Hashes: FixtureHashes{Legacy: hash(legacy), OldProduct: hash(product), IdentityMap: hash(mapping), Inventory: inv}}
	if err = validateSuite(s); err != nil {
		return Suite{}, fmt.Errorf("validate suite: %w", err)
	}
	return s, nil
}
