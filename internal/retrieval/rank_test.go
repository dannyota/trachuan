package retrieval

import (
	"errors"
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

func TestCap_ThreePerDocumentWithDemotedBackfill(t *testing.T) {
	a := rankFixture([]Candidate{rankCandidate(1, 1, "a", .4), rankCandidate(2, 2, "a", .4), rankCandidate(3, 3, "a", .4), rankCandidate(4, 4, "a", .4), rankCandidate(5, 5, "b", .4)}, nil)
	got, err := Rank(a, Recipe1(), 5, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 5 || got.Groups[3].Best.ChunkID != 5 || got.Groups[4].Best.ChunkID != 4 {
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
