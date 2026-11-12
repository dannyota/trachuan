package retrieval

import (
	"fmt"
	"testing"
)

func explicitFixture(primary, other int) (Arms, Recipe) {
	a, r := tailFixture()
	makeGroup := func(base Candidate, count int, start int64) ArmGroup {
		group := ArmGroup{}
		for i := 0; i < count; i++ {
			c := base
			c.ChunkID, c.GroupID = start+int64(i), start+int64(i)
			c.Group.Anchor = fmt.Sprintf("group-%d", c.GroupID)
			c.Descendants = append([]Provision(nil), c.Descendants...)
			c.Descendants[0].ID = c.GroupID
			group.Dense = append(group.Dense, c)
			group.Members = append(group.Members, c)
		}
		return group
	}
	a.Primary = makeGroup(a.Primary.Dense[0], primary, 100)
	a.NonCurrent = makeGroup(a.NonCurrent.Dense[0], other, 200)
	return a, r
}

func TestShape_ExplicitFalseReservesHalf(t *testing.T) {
	a, r := explicitFixture(20, 20)
	got, err := Shape(a, r, 20, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 10 || len(got.NonCurrent) != 10 {
		t.Fatalf("hits=%d noncurrent=%d", len(got.Hits), len(got.NonCurrent))
	}
}

func TestShape_ExplicitFalseUnusedSlotsReturn(t *testing.T) {
	for _, tc := range []struct{ primary, other, wantPrimary, wantOther int }{{20, 3, 17, 3}, {3, 20, 3, 17}} {
		a, r := explicitFixture(tc.primary, tc.other)
		got, err := Shape(a, r, 20, "dense")
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Hits) != tc.wantPrimary || len(got.NonCurrent) != tc.wantOther {
			t.Errorf("available %d/%d: hits=%d noncurrent=%d", tc.primary, tc.other, len(got.Hits), len(got.NonCurrent))
		}
	}
}

func TestShape_ExplicitFalseNoPrimaryAbstains(t *testing.T) {
	a, r := explicitFixture(0, 20)
	got, err := Shape(a, r, 20, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Abstain || got.Reason != "no_primary_hits" || len(got.NonCurrent) != 20 {
		t.Fatalf("abstain=%t reason=%s noncurrent=%d", got.Abstain, got.Reason, len(got.NonCurrent))
	}
}

func TestShape_ExplicitFalseSmallLimits(t *testing.T) {
	for _, tc := range []struct{ limit, primary, other int }{{1, 1, 0}, {3, 2, 1}} {
		a, r := explicitFixture(3, 3)
		got, err := Shape(a, r, tc.limit, "dense")
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Hits) != tc.primary || len(got.NonCurrent) != tc.other {
			t.Errorf("limit %d: hits=%d noncurrent=%d", tc.limit, len(got.Hits), len(got.NonCurrent))
		}
	}
}

func TestShape_GroupedPrimaryVacancies(t *testing.T) {
	for _, tc := range []struct {
		name           string
		explicit       bool
		grouped        int
		wantPrimary    int
		wantComplement int
	}{
		{"explicit", true, 32, 4, 4},
		{"default", false, 48, 5, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, r := explicitFixture(tc.grouped+4, 8)
			a.IncludeNonCurrent = tc.explicit
			r.HydrateMultiplier = 8
			r.DenseDepth, a.Depths.Dense = 100, 100
			first := a.Primary.Dense[0]
			for i := 1; i < tc.grouped; i++ {
				for _, candidates := range [][]Candidate{a.Primary.Dense, a.Primary.Members} {
					candidates[i].GroupID = first.GroupID
					candidates[i].Group = first.Group
					candidates[i].Descendants[0].ID = first.GroupID
				}
			}
			got, err := Shape(a, r, 8, "dense")
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Hits) != tc.wantPrimary || len(got.NonCurrent) != tc.wantComplement {
				t.Fatalf("hits=%d noncurrent=%d", len(got.Hits), len(got.NonCurrent))
			}
		})
	}
}

func TestShape_ExplicitFalseGroupedPrimaryBackfills(t *testing.T) {
	a, r := explicitFixture(36, 5)
	r.HydrateMultiplier = 8
	first := a.Primary.Dense[0]
	for i := 1; i < 32; i++ {
		for _, candidates := range [][]Candidate{a.Primary.Dense, a.Primary.Members} {
			candidates[i].GroupID = first.GroupID
			candidates[i].Group = first.Group
			candidates[i].Descendants[0].ID = first.GroupID
		}
	}
	got, err := Shape(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 4 || len(got.NonCurrent) != 4 {
		t.Fatalf("hits=%d noncurrent=%d", len(got.Hits), len(got.NonCurrent))
	}
}
