package evaluation

import (
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

func TestLoadSuiteFS_DoesNotRequireInventoryHash(t *testing.T) {
	files := fstest.MapFS{}
	for _, name := range []string{"legacy.json", "old-product.json", "identity-map.json", "fixtures.json"} {
		data, err := vnFixtures.ReadFile("testdata/vn/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: data}
	}
	if _, err := LoadSuiteFS(files, "."); err != nil {
		t.Fatalf("load without inventory hash: %v", err)
	}
}
