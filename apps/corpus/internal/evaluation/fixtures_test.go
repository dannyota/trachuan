package evaluation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func TestVNSuite_EmbeddedAndInvalidFixtures(t *testing.T) {
	suite, err := VNSuite()
	if err != nil || len(suite.Legacy) != 92 || len(suite.Rejections) != 3 {
		t.Fatalf("embedded suite: %d cases: %v", len(suite.Legacy), err)
	}
	if _, err := LoadSuiteFS(fstest.MapFS{}, "."); err == nil {
		t.Fatal("missing fixtures accepted")
	}
}

func TestSuite_RejectsChangedMapBytes(t *testing.T) {
	suite, err := VNSuite()
	if err != nil {
		t.Fatal(err)
	}
	suite.MapBytes[0] ^= 1
	if err = suite.VerifyMapBytes(); err == nil {
		t.Fatal("changed map bytes accepted")
	}
}

func TestSuite_RejectsMissingMapBytes(t *testing.T) {
	if err := (Suite{}).VerifyMapBytes(); err == nil {
		t.Fatal("missing map bytes accepted")
	}
}

func suiteFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	for _, name := range []string{"legacy.json", "old-product.json", "identity-map.json", "fixtures.json", "validity-cases.json", "development.json"} {
		data, err := vnFixtures.ReadFile("testdata/vn/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: data}
	}
	return files
}

func TestLoadSuiteFS_DoesNotRequireInventoryHash(t *testing.T) {
	if _, err := LoadSuiteFS(suiteFiles(t), "."); err != nil {
		t.Fatalf("load without inventory hash: %v", err)
	}
}

func TestLoadSuiteFS_DevelopmentFixture(t *testing.T) {
	files := suiteFiles(t)
	data := []byte(`[{"id":"new-negative","question":"Câu hỏi khác","expect_abstain":true,"tags":["negative","out_of_domain"]}]`)
	files["development.json"] = &fstest.MapFile{Data: data}
	var metadata map[string]any
	if err := json.Unmarshal(files["fixtures.json"].Data, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["development"] = map[string]any{"sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "origin": "synthetic authored fixture", "case_count": 1}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	files["fixtures.json"] = &fstest.MapFile{Data: metadataBytes}
	suite, err := LoadSuiteFS(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(suite.Development) != 1 || suite.Development[0].ID != "new-negative" || len(suite.Development[0].Tags) != 2 || suite.Hashes.Development != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("development fixture or hash lost")
	}
	if suite.Hashes.Legacy == suite.Hashes.Development || len(suite.Legacy) != 92 {
		t.Fatal("development changed legacy fixture")
	}
	delete(files, "development.json")
	if _, err = LoadSuiteFS(files, "."); err == nil {
		t.Fatal("missing development fixture accepted")
	}
}

func TestLoadDevelopment_ValidatesTagsAndCases(t *testing.T) {
	for _, tags := range []string{
		`[]`, `["unsupported"]`, `["negative","negative"]`,
	} {
		data := `[{"id":"new-negative","question":"Câu hỏi khác","expect_abstain":true,"tags":` + tags + `}]`
		if _, err := LoadDevelopment(strings.NewReader(data)); err == nil {
			t.Fatalf("invalid tags accepted: %s", tags)
		}
	}
	for _, data := range []string{`null`, `[] {}`, `[{"id":"new","question":"Câu hỏi khác","tags":["identifier"]}]`} {
		if _, err := LoadDevelopment(strings.NewReader(data)); err == nil {
			t.Fatalf("invalid cases accepted: %s", data)
		}
	}
	for _, tag := range []string{"identifier", "clause", "multi_document", "abbreviation", "missing_date", "historical", "out_of_domain", "negative"} {
		data := `[{"id":"new-negative","question":"Câu hỏi khác","expect_abstain":true,"tags":["` + tag + `"]}]`
		if _, err := LoadDevelopment(strings.NewReader(data)); err != nil {
			t.Fatalf("valid tag %s: %v", tag, err)
		}
	}
	if cases, err := LoadDevelopment(strings.NewReader(`[]`)); err != nil || len(cases) != 0 {
		t.Fatal("empty development rejected")
	}
}

func TestLoadSuiteFS_DevelopmentMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, hash, origin string
		count              int
	}{
		{"wrong hash", "changed", "authored", 0},
		{"wrong count", fmt.Sprintf("%x", sha256.Sum256([]byte("[]\n"))), "authored", 1},
		{"missing origin", fmt.Sprintf("%x", sha256.Sum256([]byte("[]\n"))), "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := suiteFiles(t)
			var metadata map[string]any
			if err := json.Unmarshal(files["fixtures.json"].Data, &metadata); err != nil {
				t.Fatal(err)
			}
			metadata["development"] = map[string]any{"sha256": tc.hash, "origin": tc.origin, "case_count": tc.count}
			data, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			files["fixtures.json"] = &fstest.MapFile{Data: data}
			if _, err = LoadSuiteFS(files, "."); err == nil {
				t.Fatal("invalid development metadata accepted")
			}
		})
	}
}
