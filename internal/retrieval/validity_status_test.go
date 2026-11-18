package retrieval

import (
	"encoding/json"
	"testing"
	"time"
)

func statusDay(year int, month time.Month, day int) *time.Time {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &t
}

func TestStamp_ServedClassAtReferenceDate(t *testing.T) {
	start := statusDay(2020, time.March, 1)
	end := statusDay(2026, time.July, 1)
	for _, stored := range []string{"in_force", "partial"} {
		for _, tc := range []struct {
			name     string
			from, to *time.Time
			date     Date
			want     string
		}{
			{"day before end", nil, end, Date{2026, time.June, 30}, stored},
			{"end date is excluded", nil, end, Date{2026, time.July, 1}, "expired"},
			{"after end", nil, end, Date{2027, time.January, 1}, "expired"},
			{"future end stays in force", nil, end, Date{2025, time.January, 1}, stored},
			{"start date is included", start, nil, Date{2020, time.March, 1}, stored},
			{"day before start", start, nil, Date{2020, time.February, 29}, "not_yet"},
			{"inside both dates", start, end, Date{2023, time.June, 15}, stored},
			{"before both dates", start, end, Date{2020, time.January, 1}, "not_yet"},
			{"after both dates", start, end, Date{2026, time.July, 1}, "expired"},
			{"same start and end day", end, end, Date{2026, time.July, 1}, "expired"},
			{"no dates", nil, nil, Date{2026, time.July, 1}, stored},
		} {
			t.Run(stored+"/"+tc.name, func(t *testing.T) {
				doc := Document{StatusClass: stored, StatusBasis: "manual_correction", IsCurrent: true, EffectiveFrom: tc.from, EffectiveTo: tc.to}
				date := tc.date
				inInterval := tc.want == stored
				got, _ := stamp(doc, Arms{Date: date})
				if got.StatusClass != tc.want || got.Current != inInterval || got.Basis != "source_status" || got.StatusBasis != "manual_correction" {
					t.Fatalf("default: %+v", got)
				}
				got, _ = stamp(doc, Arms{Date: Date{2099, time.January, 1}, AsOf: &date})
				if got.StatusClass != tc.want || got.Current != (inInterval && tc.from != nil) || got.Basis != "document_dates" || got.StatusBasis != "manual_correction" {
					t.Fatalf("as_of: %+v", got)
				}
			})
		}
	}
}

func TestStamp_OtherStoredClassesUnchanged(t *testing.T) {
	doc := Document{EffectiveFrom: statusDay(2020, time.March, 1), EffectiveTo: statusDay(2026, time.July, 1)}
	for _, stored := range []string{"expired", "not_yet", "suspended", "unknown", ""} {
		doc.StatusClass = stored
		for _, date := range []Date{{2020, time.January, 1}, {2023, time.June, 15}, {2026, time.July, 1}} {
			got, _ := stamp(doc, Arms{Date: date})
			if got.StatusClass != stored {
				t.Fatalf("%q at %s: %+v", stored, date.String(), got)
			}
			asOf := date
			got, _ = stamp(doc, Arms{Date: Date{2099, time.January, 1}, AsOf: &asOf})
			if got.StatusClass != stored {
				t.Fatalf("%q as of %s: %+v", stored, date.String(), got)
			}
		}
	}
}

func TestStamp_ClassChangesOnlyOutsideDateInterval(t *testing.T) {
	days := []*time.Time{nil, statusDay(2024, time.January, 1), statusDay(2024, time.January, 2), statusDay(2024, time.January, 3)}
	for _, stored := range []string{"in_force", "partial"} {
		for _, from := range days {
			for _, to := range days {
				if from != nil && to != nil && to.Before(*from) {
					continue
				}
				for _, day := range days[1:] {
					date := Date{day.Year(), day.Month(), day.Day()}
					eligible := (from == nil || !day.Before(*from)) && (to == nil || day.Before(*to))
					got, _ := stamp(Document{StatusClass: stored, IsCurrent: true, EffectiveFrom: from, EffectiveTo: to}, Arms{Date: date})
					if (got.StatusClass == stored) != eligible || got.Current != eligible {
						t.Fatalf("%s from %v to %v at %s: %+v", stored, from, to, date.String(), got)
					}
				}
			}
		}
	}
}

func TestStamp_AsOfIsTheReferenceDate(t *testing.T) {
	doc := Document{StatusClass: "in_force", IsCurrent: true, EffectiveFrom: statusDay(2020, time.March, 1), EffectiveTo: statusDay(2026, time.July, 1)}
	inside := Date{2023, time.June, 15}
	got, _ := stamp(doc, Arms{Date: Date{2026, time.November, 18}, AsOf: &inside})
	if got.StatusClass != "in_force" || !got.Current {
		t.Fatalf("today past the end, as_of inside: %+v", got)
	}
	ended := Date{2026, time.July, 1}
	got, _ = stamp(doc, Arms{Date: inside, AsOf: &ended})
	if got.StatusClass != "expired" || got.Current {
		t.Fatalf("today inside, as_of at the end: %+v", got)
	}
}

func TestStamp_ConflictAndGapsKeptWhenClassComesFromDates(t *testing.T) {
	doc := Document{StatusClass: "in_force", IsCurrent: true, ValidityConflict: true, EffectiveTo: statusDay(2026, time.July, 1), DerivationGaps: []string{"consolidated_status_unresolved"}}
	got, gaps := stamp(doc, Arms{Date: Date{2026, time.November, 18}})
	if got.StatusClass != "expired" || got.Current || !got.Unresolved {
		t.Fatalf("stamp: %+v", got)
	}
	kinds := map[string]bool{}
	for _, gap := range gaps {
		kinds[gap.Kind] = true
	}
	if len(gaps) != 3 || !kinds["validity_conflict"] || !kinds["consolidated_status_unresolved"] || !kinds["missing_effective_date"] {
		t.Fatalf("gaps: %+v", gaps)
	}
}

func TestDocumentValidity_ServesClassAtDate(t *testing.T) {
	doc := Document{StatusClass: "partial", IsCurrent: true, EffectiveTo: statusDay(2026, time.July, 1)}
	got, _ := DocumentValidity(doc, Date{2026, time.July, 1})
	if got.StatusClass != "expired" || got.Current {
		t.Fatalf("%+v", got)
	}
}

func TestStatusDocument_VietnamDayBoundary(t *testing.T) {
	boundary := statusDay(2026, time.July, 1)
	lastSecond := time.Date(2026, time.June, 30, 16, 59, 59, 0, time.UTC)
	firstSecond := lastSecond.Add(time.Second)
	for _, tc := range []struct {
		name    string
		source  Document
		now     time.Time
		class   string
		current bool
	}{
		{"before the end", Document{StatusClass: "in_force", IsCurrent: true, EffectiveTo: boundary}, lastSecond, "in_force", true},
		{"at the end", Document{StatusClass: "in_force", IsCurrent: true, EffectiveTo: boundary}, firstSecond, "expired", false},
		{"before the start", Document{StatusClass: "partial", IsCurrent: true, EffectiveFrom: boundary}, lastSecond, "not_yet", false},
		{"at the start", Document{StatusClass: "partial", IsCurrent: true, EffectiveFrom: boundary}, firstSecond, "partial", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{options: Options{Now: func() time.Time { return tc.now }}}
			var doc DocumentEntry
			e.statusDocument(&doc, tc.source)
			if doc.Status.Class != tc.class || doc.Status.Current != tc.current || doc.Status.Basis != "source_status" {
				t.Fatalf("status: %+v", doc.Status)
			}
		})
	}
}

func TestStatusDocument_CachedSourceKeepsDates(t *testing.T) {
	raw, err := json.Marshal(cachedSequence{Source: Document{StatusClass: "in_force", IsCurrent: true, EffectiveTo: statusDay(2026, time.July, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	var stored cachedSequence
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	e := &Engine{options: Options{Now: func() time.Time { return time.Date(2026, time.November, 18, 0, 0, 0, 0, time.UTC) }}}
	var doc DocumentEntry
	e.statusDocument(&doc, stored.Source)
	if doc.Status.Class != "expired" || doc.Status.Current {
		t.Fatalf("status: %+v", doc.Status)
	}
}

func TestShape_ExpiredByDateReachesHitAndPresentedDocument(t *testing.T) {
	a, recipe := shapeFixture("Nội dung")
	end := statusDay(2024, time.July, 1)
	for i := range a.Primary.Dense {
		a.Primary.Dense[i].Document.EffectiveTo = end
	}
	for i := range a.Primary.Members {
		a.Primary.Members[i].Document.EffectiveTo = end
	}
	a.NonCurrent, a.Primary = a.Primary, ArmGroup{}
	a.IncludeNonCurrent = true
	got, err := Shape(a, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NonCurrent) != 1 || got.NonCurrent[0].Stamp.StatusClass != "expired" || got.NonCurrent[0].Stamp.Current {
		t.Fatalf("response: %+v", got.NonCurrent)
	}
	presented := present(got, map[string]DocumentEntry{"vn:1": {Title: "Title"}}, DetailCompact)
	if status := presented.Documents["vn:1"].Status; status.Class != "expired" || status.Current {
		t.Fatalf("presented status: %+v", status)
	}
}
