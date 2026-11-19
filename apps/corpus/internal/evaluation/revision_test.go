package evaluation

import (
	"reflect"
	"strings"
	"testing"
)

func fixtureCase(id, question string, citations int, tags ...string) Case {
	c := Case{ID: id, Question: question, Tags: tags}
	for i := 0; i < citations; i++ {
		c.ExpectedCitations = append(c.ExpectedCitations, Expectation{DocNumber: "doc"})
	}
	c.ExpectAbstain = citations == 0
	if len(c.Tags) == 0 {
		c.Tags = []string{"identifier"}
	}
	return c
}

func TestCompareFixtures_ComparableSetAndReasons(t *testing.T) {
	previous := []Case{
		fixtureCase("a", "qa", 1),
		fixtureCase("b", "qb", 1),
		fixtureCase("c", "qc", 1),
		fixtureCase("d", "qd", 0, "negative"),
		fixtureCase("e", "qe", 1),
		fixtureCase("f", "qf", 1),
		fixtureCase("i", "qi", 1, "historical"),
		fixtureCase("j", "qj", 1, "historical"),
	}
	current := []Case{
		fixtureCase("g", "qg", 1),
		fixtureCase("a", "qa", 1, "identifier", "transition"),
		fixtureCase("b", "qb changed", 1),
		fixtureCase("c", "qc", 2),
		fixtureCase("d", "qd", 0, "negative"),
		fixtureCase("e", "qe", 1, "historical"),
		fixtureCase("f", "qf changed", 1, "historical"),
		fixtureCase("h", "qh", 1, "historical"),
		fixtureCase("i", "qi", 1),
		fixtureCase("j", "qj", 1, "historical", "multi_document"),
	}
	current[4].Notes = "notes are ignored"
	cohorts := map[string]string{"a": "development", "b": "development", "c": "development", "d": "development", "e": "development", "f": "development", "i": "development_period", "j": "development_period"}
	got, err := compareFixtures(previous, current, cohorts)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "d"}; !reflect.DeepEqual(got.comparable, want) {
		t.Fatalf("comparable = %v, want %v", got.comparable, want)
	}
	if want := []string{"a", "d"}; !reflect.DeepEqual(got.previousOrder, want) {
		t.Fatalf("previous order = %v, want %v", got.previousOrder, want)
	}
	want := []RevisedCase{{"b", "changed"}, {"c", "changed"}, {"e", "cohort"}, {"f", "cohort"}, {"g", "added"}, {"i", "cohort"}}
	if !reflect.DeepEqual(got.revised, want) {
		t.Fatalf("revised = %v, want %v", got.revised, want)
	}
}

func TestCompareFixtures_FixtureOrder(t *testing.T) {
	previous := []Case{fixtureCase("b", "qb", 1), fixtureCase("a", "qa", 1)}
	current := []Case{fixtureCase("a", "qa", 1), fixtureCase("b", "qb", 1)}
	got, err := compareFixtures(previous, current, map[string]string{"a": "development", "b": "development"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.comparable, []string{"a", "b"}) || !reflect.DeepEqual(got.previousOrder, []string{"b", "a"}) || len(got.revised) != 0 {
		t.Fatalf("revision = %+v", got)
	}
}

func TestCompareFixtures_RefusesRemoval(t *testing.T) {
	previous := []Case{fixtureCase("a", "qa", 1), fixtureCase("b", "qb", 1)}
	for name, current := range map[string][]Case{
		"removal": {fixtureCase("a", "qa", 1)},
		"rename":  {fixtureCase("a", "qa", 1), fixtureCase("b2", "qb", 1)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := compareFixtures(previous, current, map[string]string{"a": "development", "b": "development"})
			if err == nil || !strings.Contains(err.Error(), "removes case b") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCompareFixtures_RefusesInconsistentPreviousCohorts(t *testing.T) {
	previous := []Case{fixtureCase("a", "qa", 1), fixtureCase("b", "qb", 1)}
	for name, cohorts := range map[string]map[string]string{
		"missing case": {"a": "development"},
		"extra case":   {"a": "development", "b": "development", "z": "development"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := compareFixtures(previous, previous, cohorts); err == nil {
				t.Fatal("inconsistent previous report accepted")
			}
		})
	}
}

func TestVNFixtures_AdoptionRevision(t *testing.T) {
	suite, err := VNSuite()
	if err != nil {
		t.Fatal(err)
	}
	const previousHash = "19d5e1af2b7031315db7a5164d28eda38d68ee7be364d8a332caca6f72649abf"
	data, ok := suite.PreviousDevelopment[previousHash]
	if !ok {
		t.Fatal("previous development fixture missing")
	}
	previous, err := loadPreviousDevelopment(suite, previousHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(previous) != 65 || len(data) == 0 {
		t.Fatalf("previous fixture: %d cases", len(previous))
	}
	cohorts := map[string]string{}
	for _, c := range previous {
		cohorts[c.ID] = "development"
	}
	got, err := compareFixtures(previous, suite.Development, cohorts)
	if err != nil {
		t.Fatal(err)
	}
	var want []RevisedCase
	for _, id := range []string{"dev-018", "dev-022", "dev-033", "dev-039", "dev-052", "dev-070", "dev-088"} {
		want = append(want, RevisedCase{id, "cohort"})
	}
	if len(got.comparable) != 58 || !reflect.DeepEqual(got.revised, want) {
		t.Fatalf("comparable %d, revised %v", len(got.comparable), got.revised)
	}
}
