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
