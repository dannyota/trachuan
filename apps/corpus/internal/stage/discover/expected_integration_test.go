//go:build integration

package discover

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type listingFixture struct {
	rel                string
	agencies           []string
	keywords           []string
	state              string
	unstable, accepted bool
	items              map[string][]string
}

func expectedFixture(t *testing.T, fixtures ...listingFixture) (*pgxpool.Pool, stage.Run) {
	t.Helper()
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var current stage.Run
	for _, f := range fixtures {
		list := source.List{Country: "vn", VBPL: source.VBPLSources{SweepAgencies: []string{"62"}, KeywordAgencies: f.agencies, Keywords: f.keywords}}
		data, err := json.Marshal(list)
		if err != nil {
			t.Fatal(err)
		}
		current = stage.Run{Country: "vn", Rel: f.rel, Snapshot: stage.Snapshot{SourceList: data}}
		snapshot, err := json.Marshal(current.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn',$1,'paused',$2) RETURNING id`, f.rel, snapshot).Scan(&current.ID); err != nil {
			t.Fatal(err)
		}
		definitions, err := queries(current)
		if err != nil {
			t.Fatal(err)
		}
		for key, definition := range definitions {
			identity, err := queryIdentity(key, definition)
			if err != nil {
				t.Fatal(err)
			}
			state := f.state
			if state == "" {
				state = "done"
			}
			if _, err := db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch) VALUES ($1,'discover',$2,$3,0)`, current.ID, key, state); err != nil {
				t.Fatal(err)
			}
			if f.unstable {
				if _, err := db.Exec(ctx, `INSERT INTO gap (run_id,stage,item_key,kind,detail,accepted_at) VALUES ($1,'discover',$2,'listing_unstable','3', CASE WHEN $3 THEN now() ELSE NULL END)`, current.ID, key, f.accepted); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range f.items[key] {
				if _, err := db.Exec(ctx, `INSERT INTO listing (run_id,query_key,item_key,payload) VALUES ($1,$2,$3,'{}')`, current.ID, identity, "vn:vbpl:"+id); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return db, current
}

func checkExpected(t *testing.T, db *pgxpool.Pool, r stage.Run, want []Expected) {
	t.Helper()
	got, err := ExpectedSet(context.Background(), db, r)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpectedSet = %#v, want %#v", got, want)
	}
}

func fixture(rel string, ids ...string) listingFixture {
	return listingFixture{rel: rel, agencies: []string{"1"}, keywords: []string{"law"}, items: map[string][]string{"vbpl:keyword:law": ids}}
}

func document(id, gap string) Expected {
	key := "vn:vbpl:" + id
	result := Expected{Key: key, VBPLID: id}
	if gap != "" {
		result.Skip = &stage.Gap{Kind: gap, Detail: key}
	}
	return result
}

func TestExpectedSet_WithdrawnAfterTwoAbsences(t *testing.T) {
	t.Parallel()
	db, r := expectedFixture(t, fixture("20261101", "1"), fixture("20261102"), fixture("20261103"))
	checkExpected(t, db, r, []Expected{document("1", "withdrawn")})
}

func TestExpectedSet_OneAbsenceKeepsDocument(t *testing.T) {
	t.Parallel()
	db, r := expectedFixture(t, fixture("20261101", "1"), fixture("20261102"))
	checkExpected(t, db, r, []Expected{document("1", "")})
}

func TestExpectedSet_IncompleteListingKeepsDocument(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name, state                 string
		unstable, accepted, current bool
	}{
		{"unstable earlier", "done", true, false, false}, {"accepted unstable earlier", "done", true, true, false},
		{"failed earlier", "failed", false, false, false}, {"unstable current", "done", true, false, true},
		{"accepted unstable current", "done", true, true, true}, {"failed current", "failed", false, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			earlier, current := fixture("20261102"), fixture("20261103")
			target := &earlier
			if scenario.current {
				target = &current
			}
			target.state, target.unstable, target.accepted = scenario.state, scenario.unstable, scenario.accepted
			db, r := expectedFixture(t, fixture("20261101", "1"), earlier, current)
			checkExpected(t, db, r, []Expected{document("1", "")})
		})
	}
}

func TestExpectedSet_QueryDefinitionChangeIsNotComparable(t *testing.T) {
	t.Parallel()
	old, middle, current := fixture("20261101", "1"), fixture("20261102"), fixture("20261103")
	middle.agencies, current.agencies = []string{"2"}, []string{"2"}
	db, r := expectedFixture(t, old, middle, current)
	checkExpected(t, db, r, []Expected{document("1", "scope_removed")})
}

func TestExpectedSet_OrdersRelsNumerically(t *testing.T) {
	t.Parallel()
	db, r := expectedFixture(t, fixture("20261110_1", "1"), fixture("20261110_10"), fixture("20261110_2", "1"), fixture("20261111"))
	checkExpected(t, db, r, []Expected{document("1", "withdrawn")})
}

func TestExpectedSet_ScopeRemovedIsSkipGap(t *testing.T) {
	t.Parallel()
	current := fixture("20261102")
	current.keywords = nil
	current.items["vbpl:sweep"] = []string{"2"}
	db, r := expectedFixture(t, fixture("20261101", "1"), current)
	checkExpected(t, db, r, []Expected{document("1", "scope_removed"), document("2", "")})
}

func TestExpectedSet_AllListingQueriesMustBeAbsentTwice(t *testing.T) {
	t.Parallel()
	first, middle, current := fixture("20261101", "1"), fixture("20261102"), fixture("20261103")
	first.items["vbpl:sweep"] = []string{"1"}
	middle.items["vbpl:sweep"] = []string{"1"}
	db, r := expectedFixture(t, first, middle, current)
	checkExpected(t, db, r, []Expected{document("1", "")})
}

func TestExpectedSet_CurrentUnstableUnionIsExpected(t *testing.T) {
	t.Parallel()
	current := fixture("20261101", "2", "1")
	current.unstable = true
	db, r := expectedFixture(t, current)
	checkExpected(t, db, r, []Expected{document("1", ""), document("2", "")})
}

func TestExpectedSet_SkipsEnglishRenditions(t *testing.T) {
	t.Parallel()
	db, r := expectedFixture(t, fixture("20261101", "1", "vbpqta_3188"))
	checkExpected(t, db, r, []Expected{document("1", "")})
}
