package relate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestPairInputHash_Vectors(t *testing.T) {
	for _, tc := range []struct{ extract, want string }{
		{"def", "713c06e158bcfff763fd126d63e0dc3fee018ac10e0cae28ed882b97e21bb5a0"},
		{"-", "8c2b9340dcb652dc6767e2d9b319ea3ea12984afe94a5e310acc07e5da2a342b"},
	} {
		if got := PairInputHash("abc", tc.extract); got != tc.want {
			t.Errorf("extract %q: got %s, want %s", tc.extract, got, tc.want)
		}
	}
}

func TestExtractHash_TerminalStates(t *testing.T) {
	hash := "extract"
	for _, tc := range []struct {
		state    string
		artifact *string
		want     string
		finished bool
	}{
		{"done", &hash, hash, true}, {"gap", nil, "-", true}, {"failed", nil, "-", true},
		{"pending", nil, "", false}, {"running", nil, "", false}, {"done", nil, "", false},
	} {
		t.Run(tc.state, func(t *testing.T) {
			got, finished := extractHash(store.GetRunItemCommittedRow{State: tc.state, ArtifactSha256: tc.artifact})
			if got != tc.want || finished != tc.finished {
				t.Fatalf("got %q, %v", got, finished)
			}
		})
	}
}

func testRelations(t *testing.T, detail, diagram string, codes map[string]source.RelationCode) ([]relation, []string) {
	t.Helper()
	rows, unmapped, err := relations([]byte(detail), []byte(diagram), codes)
	if err != nil {
		t.Fatal(err)
	}
	return rows, unmapped
}

func TestRelate_MapsCodes(t *testing.T) {
	rows, _ := testRelations(t, `{"data":{"references":[{"referenceType":10,"targetDocument":{"id":"42","docNum":"1/2020/TT","title":"Title"}}]}}`, `{"data":{}}`, map[string]source.RelationCode{"10": {Kind: "amends_supplements", Label: "Amends"}})
	if len(rows) != 1 || rows[0].Kind != "amends_supplements" || rows[0].RawCode != "10" || rows[0].RawLabel != "Amends" || rows[0].Evidence != "Title" {
		t.Fatalf("%+v", rows)
	}
}

func TestRelate_MergesReferencesAndDiagram(t *testing.T) {
	rows, _ := testRelations(t, `{"data":{"references":[{"referenceType":10,"targetDocument":{"id":"42","docNum":"1/2020/TT","title":"Title"}}]}}`, `{"data":{"documentNamesByType":{"10":[{"id":"42","name":"Document 1/2020/TT"}]}}}`, nil)
	if len(rows) != 1 || rows[0].Method != "vbpl_references,vbpl_diagram" || rows[0].TargetNumber != "1/2020/TT" {
		t.Fatalf("%+v", rows)
	}
}

func TestRelate_UnmappedCodeIsGap(t *testing.T) {
	rows, codes := testRelations(t, `{"data":{"references":[]}}`, `{"data":{"documentNamesByType":{"99":[{"id":"42","name":"Title"}]}}}`, nil)
	gaps := resolveGaps(rows, codes, nil)
	if len(gaps) != 2 || gaps[0] != (stage.Gap{Kind: "unknown_relation_code", Detail: "99"}) || rows[0].Kind != "unknown" {
		t.Fatalf("%+v %+v", rows, gaps)
	}
}

func TestRelate_UnknownTargetIsGap(t *testing.T) {
	rows := []relation{{TargetVBPLID: "42"}, {TargetNumber: "1/2020/TT"}}
	gaps := resolveGaps(rows, nil, map[string]bool{"vn:vbpl:43": true})
	if len(gaps) != 2 || gaps[0].Detail != "42" || gaps[1].Detail != "1/2020/TT" || gaps[0].Kind != "unknown_target" || gaps[1].Kind != "unknown_target" {
		t.Fatalf("%+v", gaps)
	}
}

func TestRelate_ReusedArtifactResolvesAgainstNewExpectedSet(t *testing.T) {
	rows := []relation{{TargetVBPLID: "42"}}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var reused []relation
	if err = json.Unmarshal(data, &reused); err != nil {
		t.Fatal(err)
	}
	if len(resolveGaps(reused, nil, nil)) != 1 || len(resolveGaps(reused, nil, map[string]bool{"vn:vbpl:42": true})) != 0 {
		t.Fatal("resolution did not use current expected set")
	}
}

func TestRelate_CodeTableChangeChangesRecipe(t *testing.T) {
	a := stage.Snapshot{SourceList: json.RawMessage(`{"relation_codes":{"10":{"kind":"amends_supplements"}}}`)}
	b := stage.Snapshot{SourceList: json.RawMessage(`{"relation_codes":{"10":{"kind":"replaces"}}}`)}
	if New().Recipe(a) == New().Recipe(b) {
		t.Fatal("recipe did not change")
	}
}

func TestRelate_IssuerTableChangesRecipe(t *testing.T) {
	a := stage.Snapshot{SourceList: json.RawMessage(`{"central_issuers":{"CP":"CHÍNH PHỦ"}}`)}
	b := stage.Snapshot{SourceList: json.RawMessage(`{"central_issuers":{"CP":"OTHER"}}`)}
	if New().Recipe(a) == New().Recipe(b) {
		t.Fatal("recipe did not change")
	}
}

func TestRelate_RecipeForSnapshotWithoutIssuerTable(t *testing.T) {
	snapshot := stage.Snapshot{SourceList: json.RawMessage(`{"relation_codes":{"10":{"kind":"amends_supplements"}}}`)}
	list, err := source.Parse(snapshot.SourceList)
	if err != nil {
		t.Fatal(err)
	}
	want := stage.RecipeHash("relate", "vbpl-relations/1", "chinhphu-empty/1", list.RelationCodesHash(), "text_rules:vn-targets/1", list.CentralIssuersHash())
	if got := New().Recipe(snapshot); got != want {
		t.Fatalf("recipe = %s, want %s", got, want)
	}
}

func TestRelate_DiagramFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "diagram-171687.json"))
	if err != nil {
		t.Fatal(err)
	}
	rows, codes, err := relations([]byte(`{"data":{"references":null}}`), data, map[string]source.RelationCode{"1": {Kind: "one"}, "3": {Kind: "three"}, "12": {Kind: "twelve"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 9 || len(codes) != 0 || rows[0].TargetNumber != "09/2020/TT-NHNN" || rows[1].RawCode != "12" || rows[3].RawCode != "3" {
		t.Fatalf("rows=%+v codes=%v", rows, codes)
	}
}
