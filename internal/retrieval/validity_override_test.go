package retrieval

import (
	"context"
	"reflect"
	"testing"
)

func TestValidityOverrides_EvalOnly(t *testing.T) {
	requestType := reflect.TypeOf(Request{})
	for _, field := range []string{"HistoricalPolicy", "NonCurrentTail", "NotYetEligible"} {
		if _, ok := requestType.FieldByName(field); ok {
			t.Errorf("serving request exposes %s", field)
		}
	}
	engine := &Engine{options: Options{Profile: "serving"}}
	if _, err := engine.CollectWithNotYetEligible(context.Background(), Prepared{}, Depths{}); err == nil {
		t.Fatal("serving profile accepted research eligibility")
	}
	_, recipe := tailFixture()
	hash := recipe.Hash()
	a, _ := tailFixture()
	if _, err := ShapeWithNonCurrentTail(a, recipe, 3, "dense", nil, 1); err != nil {
		t.Fatal(err)
	}
	if recipe.Hash() != hash {
		t.Fatal("research shaping changed the recipe hash")
	}
}

func tailFixture() (Arms, Recipe) {
	a, recipe := shapeFixture("evidence")
	first := a.Primary.Dense[0]
	for i := int64(2); i <= 3; i++ {
		c := first
		c.ChunkID, c.GroupID = i, i+1
		c.Group.Anchor = string(rune('a' + i))
		c.Descendants = append([]Provision(nil), c.Descendants...)
		c.Descendants[0].ID = c.GroupID
		a.Primary.Dense = append(a.Primary.Dense, c)
		a.Primary.Members = append(a.Primary.Members, c)
	}
	other := first
	other.ChunkID, other.GroupID = 10, 10
	other.Group.Anchor = "expired"
	other.Descendants = append([]Provision(nil), other.Descendants...)
	other.Descendants[0].ID = other.GroupID
	other.Document.StatusClass = "expired"
	other.Document.IsCurrent = false
	a.NonCurrent = ArmGroup{Dense: []Candidate{other}, Members: []Candidate{other}}
	a.IncludeNonCurrent = true
	return a, recipe
}

func TestNonCurrentTail_AppendsMarkedComplementHits(t *testing.T) {
	a, recipe := tailFixture()
	got, err := ShapeWithNonCurrentTail(a, recipe, 3, "dense", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 2 || len(got.NonCurrent) != 1 || got.NonCurrent[0].Stamp.Current || got.NonCurrent[0].Stamp.StatusClass != "expired" {
		t.Fatalf("response: %+v", got)
	}
}

func TestNonCurrentTail_PrimaryKeepsFreedSlots(t *testing.T) {
	a, recipe := tailFixture()
	a.NonCurrent = ArmGroup{}
	got, err := ShapeWithNonCurrentTail(a, recipe, 3, "dense", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 3 || len(got.NonCurrent) != 0 {
		t.Fatalf("response: %+v", got)
	}
}

func TestNonCurrentTail_UsesFullLimitForComplementShaping(t *testing.T) {
	a, recipe := tailFixture()
	recipe.HydrateMultiplier = 1
	other := a.NonCurrent.Dense[0]
	a.NonCurrent = ArmGroup{}
	for i := int64(1); i <= 5; i++ {
		c := other
		c.ChunkID, c.GroupID = i+10, i+10
		c.Group.Anchor = string(rune('a' + i))
		if i == 5 {
			c.Group.DocumentKey = "vn:other"
		}
		c.Descendants = append([]Provision(nil), c.Descendants...)
		c.Descendants[0].ID = c.GroupID
		a.NonCurrent.Dense = append(a.NonCurrent.Dense, c)
		a.NonCurrent.Members = append(a.NonCurrent.Members, c)
	}
	got, err := ShapeWithNonCurrentTail(a, recipe, 8, "dense", nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NonCurrent) != 4 || got.NonCurrent[3].Identity.DocumentKey != "vn:other" {
		t.Fatalf("complement: %+v", got.NonCurrent)
	}
}

func TestNonCurrentTail_NoPrimaryStillAbstains(t *testing.T) {
	a, recipe := tailFixture()
	a.Primary = ArmGroup{}
	got, err := ShapeWithNonCurrentTail(a, recipe, 3, "dense", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Abstain || got.Reason != "no_primary_hits" {
		t.Fatalf("response: %+v", got)
	}
}
