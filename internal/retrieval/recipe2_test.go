package retrieval

import "testing"

func TestRecipe2_CapAndTailValues(t *testing.T) {
	r := Recipe1()
	if r.Version != 2 || r.DocumentCap != 5 || r.NonCurrentTail != 2 {
		t.Fatalf("recipe: %+v", r)
	}
}

func TestRecipeHash_ChangesWithTail(t *testing.T) {
	r := Recipe1()
	before := r.Hash()
	r.NonCurrentTail = 0
	if r.Hash() == before {
		t.Fatal("tail did not change recipe hash")
	}
}

func TestShape_DefaultModeReservesTail(t *testing.T) {
	a, r := tailFixture()
	a.IncludeNonCurrent = false
	other := a.NonCurrent.Dense[0]
	other.ChunkID, other.GroupID = 11, 11
	other.Group.Anchor = "second"
	other.Descendants = append([]Provision(nil), other.Descendants...)
	other.Descendants[0].ID = other.GroupID
	a.NonCurrent.Dense = append(a.NonCurrent.Dense, other)
	a.NonCurrent.Members = append(a.NonCurrent.Members, other)
	got, err := Shape(a, r, 4, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 2 || len(got.NonCurrent) != 2 || got.NonCurrent[0].Stamp.Current || got.NonCurrent[1].Stamp.Current {
		t.Fatalf("response: %+v", got)
	}
}

func TestShape_DefaultModeFreedSlots(t *testing.T) {
	a, r := tailFixture()
	a.IncludeNonCurrent = false
	got, err := Shape(a, r, 3, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 2 || len(got.NonCurrent) != 1 {
		t.Fatalf("response: %+v", got)
	}
}

func TestShape_NoPrimaryAbstains(t *testing.T) {
	a, r := tailFixture()
	a.IncludeNonCurrent = false
	a.Primary = ArmGroup{}
	got, err := Shape(a, r, 3, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Abstain || got.Reason != "no_primary_hits" || len(got.NonCurrent) != 1 {
		t.Fatalf("response: %+v", got)
	}
}

func TestShape_NotYetNeverPrimary(t *testing.T) {
	a, r := tailFixture()
	a.IncludeNonCurrent = false
	a.NonCurrent.Dense[0].Document.StatusClass = "not_yet"
	a.NonCurrent.Members[0].Document.StatusClass = "not_yet"
	got, err := Shape(a, r, 3, "dense")
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range got.Hits {
		if hit.Stamp.StatusClass == "not_yet" {
			t.Fatal("not-yet primary")
		}
	}
	if len(got.NonCurrent) != 1 || got.NonCurrent[0].Stamp.StatusClass != "not_yet" || got.NonCurrent[0].Stamp.Current {
		t.Fatalf("response: %+v", got)
	}
}

func TestOverride_ReplacesRecipeTail(t *testing.T) {
	a, r := tailFixture()
	a.IncludeNonCurrent = false
	baseline, err := Shape(a, r, 3, "dense")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ShapeWithNonCurrentTail(a, r, 3, "dense", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.NonCurrent) != 1 || len(got.NonCurrent) != 0 || len(got.Hits) != 3 {
		t.Fatalf("baseline=%+v override=%+v", baseline, got)
	}
}

func TestShape_SmallLimitKeepsPrimary(t *testing.T) {
	a, r := tailFixture()
	a.IncludeNonCurrent = false
	other := a.NonCurrent.Dense[0]
	other.ChunkID, other.GroupID = 11, 11
	other.Group.Anchor = "second"
	other.Descendants = append([]Provision(nil), other.Descendants...)
	other.Descendants[0].ID = other.GroupID
	a.NonCurrent.Dense = append(a.NonCurrent.Dense, other)
	a.NonCurrent.Members = append(a.NonCurrent.Members, other)
	for _, tc := range []struct{ limit, primary, other int }{{1, 1, 0}, {2, 1, 1}, {3, 2, 1}, {4, 2, 2}} {
		for _, override := range []bool{false, true} {
			var got Response
			var err error
			if override {
				got, err = ShapeWithNonCurrentTail(a, r, tc.limit, "dense", nil, 2)
			} else {
				got, err = Shape(a, r, tc.limit, "dense")
			}
			if err != nil || got.Abstain || len(got.Hits) != tc.primary || len(got.NonCurrent) != tc.other {
				t.Fatalf("limit %d override %t: response %+v, error %v", tc.limit, override, got, err)
			}
		}
	}
	got, err := ShapeWithNonCurrentTail(a, r, 2, "dense", nil, 5)
	if err != nil || len(got.Hits) != 1 || len(got.NonCurrent) != 1 {
		t.Fatalf("large override: response %+v, error %v", got, err)
	}
	threshold := 0.9
	r.Abstention.Threshold = &threshold
	got, err = Shape(a, r, 2, "dense")
	if err != nil || !got.Abstain || got.Reason != "low_confidence" || len(got.NonCurrent) != 1 {
		t.Fatalf("low confidence: response %+v, error %v", got, err)
	}
	a.Primary = ArmGroup{}
	got, err = Shape(a, r, 1, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Abstain || got.Reason != "no_primary_hits" || len(got.NonCurrent) != 0 {
		t.Fatalf("response: %+v", got)
	}
}

func TestShape_InvalidTail(t *testing.T) {
	a, r := tailFixture()
	for _, tail := range []int{-1, 6} {
		r.NonCurrentTail = tail
		if _, err := Shape(a, r, 8, "dense"); err == nil {
			t.Fatalf("accepted tail %d", tail)
		}
	}
}
