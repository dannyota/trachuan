package retrieval

import (
	"reflect"
	"testing"
)

func TestArticleCap_Rank(t *testing.T) {
	makeCandidate := func(id int64, doc, article string) Candidate {
		c := rankCandidate(id, id, doc, .8)
		if article != "" {
			c.Ancestors = []Provision{{Kind: "dieu", Identity: Identity{Country: "vn", DocumentKey: doc, Anchor: article}}}
		}
		return c
	}
	candidates := []Candidate{makeCandidate(1, "a", "one"), makeCandidate(2, "a", "one"), makeCandidate(3, "a", "two"), makeCandidate(4, "a", ""), makeCandidate(5, "b", "one"), makeCandidate(6, "a", "")}
	for _, tc := range []struct {
		name     string
		limit    int
		document *CapOverride
		want     []int64
	}{
		{"siblings", 3, nil, []int64{1, 3, 4}},
		{"backfill", 6, nil, []int64{1, 3, 4, 5, 6, 2}},
		{"both caps", 5, func() *CapOverride { v := CapOverride(2); return &v }(), []int64{1, 3, 5, 2, 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cap := ArticleCapOverride(1)
			got, err := RankWithArticleCap(rankFixture(candidates, nil), Recipe2(), tc.limit, "hybrid", tc.document, &cap)
			if err != nil {
				t.Fatal(err)
			}
			var ids []int64
			for _, g := range got.Groups {
				ids = append(ids, g.Best.ChunkID)
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("ids = %v, want %v", ids, tc.want)
			}
		})
	}
}

func TestArticleCap_EvalOnly(t *testing.T) {
	arms, recipe := shapeFixture("text")
	beforeHash := recipe.Hash()
	before, err := Shape(arms, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	cap := ArticleCapOverride(1)
	if _, err = ShapeWithArticleCap(arms, recipe, 8, "hybrid", nil, nil, &cap); err != nil {
		t.Fatal(err)
	}
	after, err := Shape(arms, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash != recipe.Hash() || !reflect.DeepEqual(before, after) {
		t.Fatal("override changed serving or recipe")
	}
	if _, ok := reflect.TypeOf(Request{}).FieldByName("ArticleCapOverride"); ok {
		t.Fatal("serving request exposes article cap")
	}
}

func TestArticleCap_Shape(t *testing.T) {
	arms, recipe := shapeFixture("text")
	base := arms.Primary.Dense[0]
	arms.Primary = ArmGroup{}
	for i := int64(1); i <= 4; i++ {
		c := base
		c.ChunkID, c.GroupID = i, i
		c.Group.Anchor = string(rune('a' + i))
		c.Descendants = append([]Provision(nil), base.Descendants...)
		c.Descendants[0].ID = i
		if i == 4 {
			c.Ancestors = append([]Provision(nil), base.Ancestors...)
			c.Ancestors[0].Identity.Anchor = "other-article"
		}
		arms.Primary.Dense = append(arms.Primary.Dense, c)
		arms.Primary.Members = append(arms.Primary.Members, c)
	}
	one := ArticleCapOverride(1)
	got, err := ShapeWithArticleCap(arms, recipe, 2, "dense", nil, nil, &one)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 2 || got.Hits[0].Identity.Anchor != "b" || got.Hits[1].Identity.Anchor != "e" {
		t.Fatalf("hits: %+v", got.Hits)
	}
	none := ArticleCapOverride(0)
	got, err = ShapeWithArticleCap(arms, recipe, 2, "dense", nil, nil, &none)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 2 || got.Hits[1].Identity.Anchor != "c" {
		t.Fatalf("uncapped hits: %+v", got.Hits)
	}
}

func TestArticleCap_DefaultParity(t *testing.T) {
	arms, recipe := shapeFixture("text")
	base := arms.Primary.Dense[0]
	for i := int64(3); i < 9; i++ {
		c := base
		c.ChunkID, c.GroupID = i, i
		c.Descendants = append([]Provision(nil), base.Descendants...)
		c.Descendants[0].ID = i
		arms.Primary.Dense = append(arms.Primary.Dense, c)
		arms.Primary.Members = append(arms.Primary.Members, c)
	}
	normal, err := Rank(arms, recipe, 4, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	trial, err := RankWithArticleCap(arms, recipe, 4, "hybrid", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normal, trial) {
		t.Fatalf("rank differs: %+v vs %+v", normal, trial)
	}
	normalShape, err := Shape(arms, recipe, 4, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	trialShape, err := ShapeWithArticleCap(arms, recipe, 4, "hybrid", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalShape, trialShape) {
		t.Fatalf("shape differs: %+v vs %+v", normalShape, trialShape)
	}
}
