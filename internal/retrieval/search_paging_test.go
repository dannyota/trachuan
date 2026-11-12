package retrieval

import "testing"

func TestPaging_ListRanking(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	if _, err := rankWithCap(a, r, 100, "dense", r.DocumentCap); err != nil {
		t.Fatalf("internal list cap: %v", err)
	}
	if _, err := Rank(a, r, 100, "dense"); err == nil {
		t.Fatal("public ranking accepted a limit above 50")
	}
}
