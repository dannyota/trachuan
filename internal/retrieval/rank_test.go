package retrieval

import (
	"errors"
	"reflect"
	"testing"
)

func TestRank_FirstBestCosineWithoutThreshold(t *testing.T) {
	c := Candidate{ChunkID: 1, GroupID: 1, Group: Identity{DocumentKey: "vn:1"}, Cosine: 0.4}
	a := Arms{Validation: Validation{Accepted: true}, Depths: Depths{Dense: 50, Lexical: 50}, Primary: ArmGroup{Dense: []Candidate{c}, Members: []Candidate{c}}}
	r, err := Rank(a, Recipe1(), 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if r.FirstBestCosine == nil || *r.FirstBestCosine != 0.4 {
		t.Fatalf("cosine: %v", r.FirstBestCosine)
	}
}

func TestFuse_WeightedRRFWithMissingArm(t *testing.T) {
	a := rankFixture([]Candidate{rankCandidate(1, 1, "a", .4), rankCandidate(2, 2, "b", .5)}, []Candidate{rankCandidate(2, 2, "b", .5)})
	got, err := Rank(a, Recipe1(), 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 2 || got.Groups[0].Best.ChunkID != 2 || got.Groups[1].Best.ChunkID != 1 {
		t.Fatalf("groups: %+v", got.Groups)
	}
}

func TestFuse_TiesByChunkID(t *testing.T) {
	a := rankFixture([]Candidate{rankCandidate(2, 2, "b", .4)}, []Candidate{rankCandidate(1, 1, "a", .5)})
	recipe := Recipe1()
	recipe.RRF.LexicalWeight = 1
	got, err := Rank(a, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Groups[0].Best.ChunkID != 1 {
		t.Fatalf("groups: %+v", got.Groups)
	}
}

func TestFuse_RoutedLexicalWeight(t *testing.T) {
	a := rankFixture([]Candidate{rankCandidate(2, 2, "b", .4)}, []Candidate{rankCandidate(1, 1, "a", .5)})
	a.Validation.Routed = true
	got, err := Rank(a, Recipe1(), 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Groups[0].Best.ChunkID != 1 {
		t.Fatalf("groups: %+v", got.Groups)
	}
}

func TestGroup_MaxReducerNeverSums(t *testing.T) {
	a := rankFixture([]Candidate{rankCandidate(1, 1, "a", .4), rankCandidate(2, 2, "b", .5), rankCandidate(3, 2, "b", .9)}, nil)
	got, err := Rank(a, Recipe1(), 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 2 || got.Groups[0].Best.ChunkID != 1 || got.Groups[1].BestCosine != .9 {
		t.Fatalf("groups: %+v", got.Groups)
	}
}

func TestCap_FivePerDocumentWithDemotedBackfill(t *testing.T) {
	// Recipe 2 raises the diversity cap to five.
	a := rankFixture([]Candidate{rankCandidate(1, 1, "a", .4), rankCandidate(2, 2, "a", .4), rankCandidate(3, 3, "a", .4), rankCandidate(4, 4, "a", .4), rankCandidate(5, 5, "a", .4), rankCandidate(6, 6, "a", .4), rankCandidate(7, 7, "b", .4)}, nil)
	got, err := Rank(a, Recipe1(), 7, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 7 || got.Groups[5].Best.ChunkID != 7 || got.Groups[6].Best.ChunkID != 6 {
		t.Fatalf("groups: %+v", got.Groups)
	}
}

func rankCandidate(id, group int64, doc string, cosine float64) Candidate {
	return Candidate{ChunkID: id, GroupID: group, Group: Identity{DocumentKey: doc}, Cosine: cosine}
}

func rankFixture(dense, lexical []Candidate) Arms {
	members := append(append([]Candidate{}, dense...), lexical...)
	return Arms{Validation: Validation{Accepted: true}, Depths: Depths{Dense: 50, Lexical: 50}, Primary: ArmGroup{Dense: dense, Lexical: lexical, Members: members}}
}

func TestRank_CacheInsufficientFails(t *testing.T) {
	a := rankFixture([]Candidate{rankCandidate(1, 1, "a", .4)}, nil)
	a.Primary.Members = nil
	_, err := Rank(a, Recipe1(), 8, "hybrid")
	if !errors.Is(err, ErrCacheInsufficient) {
		t.Fatalf("got %v, want cache insufficient", err)
	}
}

func TestRank_SingleArmRemovesOtherArmBeforeFusion(t *testing.T) {
	a := rankFixture([]Candidate{rankCandidate(1, 1, "a", .4)}, []Candidate{rankCandidate(2, 2, "b", .5)})
	got, err := Rank(a, Recipe1(), 8, "lexical")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Best.ChunkID != 2 {
		t.Fatalf("groups: %+v", got.Groups)
	}
}

func TestGroup_PointRollsUpToNearestClause(t *testing.T) {
	clause := rankCandidate(1, 12, "a", .4)
	clause.Group = Identity{DocumentKey: "a", Anchor: "clause"}
	point := rankCandidate(2, 12, "a", .9)
	point.Group = clause.Group
	a := rankFixture([]Candidate{clause, point}, nil)
	got, err := Rank(a, Recipe1(), 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Identity.Anchor != "clause" || got.Groups[0].BestCosine != .9 {
		t.Fatalf("groups %+v", got.Groups)
	}
}
func TestGroup_ArticleOwnedStaysArticle(t *testing.T) {
	article := rankCandidate(1, 11, "a", .4)
	article.Group = Identity{DocumentKey: "a", Anchor: "article"}
	clause := rankCandidate(2, 12, "a", .5)
	clause.Group = Identity{DocumentKey: "a", Anchor: "clause"}
	got, err := Rank(rankFixture([]Candidate{article, clause}, nil), Recipe1(), 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 2 || got.Groups[0].Identity.Anchor != "article" {
		t.Fatalf("groups %+v", got.Groups)
	}
}

func TestRank_CapOverrideNone(t *testing.T)  { testCapOverride(t, "none", 5) }
func TestRank_CapOverrideValue(t *testing.T) { testCapOverride(t, "4", 4) }

func testCapOverride(t *testing.T, value string, want int) {
	t.Helper()
	var candidates []Candidate
	for i := int64(1); i <= 5; i++ {
		candidates = append(candidates, rankCandidate(i, i, "a", .4))
	}
	for i := int64(6); i <= 10; i++ {
		candidates = append(candidates, rankCandidate(i, i, "b", .4))
	}
	cap, err := ParseCapOverride(value)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Rank(rankFixture(candidates, nil), Recipe1(), 5, "hybrid", cap)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, g := range got.Groups {
		if g.Identity.DocumentKey == "a" {
			count++
		}
	}
	if count != want {
		t.Fatalf("document hits = %d, want %d", count, want)
	}
}

func TestCapOverride_EvalOnly(t *testing.T) {
	recipe := Recipe1()
	before := recipe.Hash()
	cap, err := ParseCapOverride("none")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Rank(rankFixture(nil, nil), recipe, 8, "hybrid", cap)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.Hash() != before || Recipe1().Hash() != before {
		t.Fatal("recipe hash changed")
	}
	if _, ok := reflect.TypeOf(Request{}).FieldByName("CapOverride"); ok {
		t.Fatal("serving request exposes cap override")
	}
}

func TestShapeWithCap_AppliesOverride(t *testing.T) {
	arms, recipe := shapeFixture("text")
	base := arms.Primary.Dense[0]
	arms.Primary = ArmGroup{}
	for i := int64(1); i <= 10; i++ {
		c := base
		c.ChunkID, c.GroupID = i, i
		c.Descendants = append([]Provision(nil), base.Descendants...)
		c.Descendants[0].ID = i
		if i > 5 {
			c.Group.DocumentKey = "other"
		}
		arms.Primary.Dense = append(arms.Primary.Dense, c)
		arms.Primary.Members = append(arms.Primary.Members, c)
	}
	cap, err := ParseCapOverride("4")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ShapeWithCap(arms, recipe, 5, "hybrid", cap)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 5 || got.Hits[3].Identity.DocumentKey != "vn:1" || got.Hits[4].Identity.DocumentKey != "other" {
		t.Fatalf("hits: %+v", got.Hits)
	}
}
