package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVNFixtures_Validate(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "vn", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	legacy := read("legacy.json")
	cases, err := LoadLegacy(strings.NewReader(string(legacy)))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SourcePath       string `json:"source_path"`
		SourceRepository string `json:"source_repository"`
		SourceCommit     string `json:"source_commit"`
		SHA256           string `json:"sha256"`
		Origin           string `json:"origin"`
		CaseCount        int    `json:"case_count"`
	}
	if err = json.Unmarshal(read("fixtures.json"), &fixture); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(legacy)
	hash := hex.EncodeToString(sum[:])
	if fixture.SourcePath != "deploy/eval/golden.json" || fixture.SourceRepository != "banhmi" || fixture.SourceCommit != "6c3c531d29a5e97080d0e3415269d6a642c04f19" || fixture.SHA256 != hash || hash != "881d09074ed5160e1e3ef97cd9220eb53ee16ccbbd7d7b0d39fb9483bbcb3aae" || fixture.Origin != "authored evaluation cases, not user questions" || fixture.CaseCount != 92 {
		t.Fatal("invalid fixture provenance")
	}
	var product struct {
		LegacySHA256 string   `json:"legacy_sha256"`
		RejectionIDs []string `json:"rejection_ids"`
	}
	if err = json.Unmarshal(read("old-product.json"), &product); err != nil {
		t.Fatal(err)
	}
	if product.LegacySHA256 != hash || strings.Join(product.RejectionIDs, ",") != "edge-no-diacritics-infosec,edge-no-diacritics-payment,edge-no-diacritics-mangluoi" {
		t.Fatal("invalid old product fixture")
	}
	m, err := LoadIdentityMap(strings.NewReader(string(read("identity-map.json"))))
	if err != nil {
		t.Fatal(err)
	}
	expectations, negatives := 0, 0
	for _, c := range cases {
		expectations += len(c.ExpectedCitations)
		if c.ExpectAbstain {
			negatives++
		}
	}
	if len(cases) != 92 || expectations != 96 || negatives != 5 || len(m.Entries) != 96 {
		t.Fatalf("counts: cases=%d expectations=%d negatives=%d entries=%d", len(cases), expectations, negatives, len(m.Entries))
	}
	report, err := m.Validate(cases, hash, IdentityInventory{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report) == 0 {
		t.Fatal("missing inventory should report incomplete mappings")
	}
}
