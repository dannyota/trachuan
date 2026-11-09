package source

import (
	"os"
	"testing"
)

func TestParseVN(t *testing.T) {
	data, err := os.ReadFile("../../../../sources/vn.yaml")
	if err != nil {
		t.Fatal(err)
	}
	list, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if list.Country != "vn" || len(list.VBPL.SweepAgencies) != 2 || len(list.VBPL.KeywordAgencies) != 8 || len(list.VBPL.Keywords) != 26 || len(list.RelationCodes) != 11 || len(list.StatusCodes) != 8 {
		t.Fatalf("unexpected source list: %+v", list)
	}
}

func TestParse_JSONSnapshot(t *testing.T) {
	list, err := Parse([]byte(`{"country":"vn","vbpl":{"sweep_agencies":["62"]},"relation_codes":{"1":{"kind":"repeals","label":"repeals","amending":true,"superseding":true}},"status_codes":{"CHL":{"class":"in_force","current":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if list.Country != "vn" || len(list.VBPL.SweepAgencies) != 1 || !list.RelationCodes["1"].Amending || !list.StatusCodes["CHL"].Current {
		t.Fatalf("unexpected source list: %+v", list)
	}
}

func TestRelationCodesHash_OrderIndependent(t *testing.T) {
	a := List{RelationCodes: map[string]RelationCode{"1": {Kind: "one"}, "2": {Kind: "two"}}}
	b := List{RelationCodes: map[string]RelationCode{"2": {Kind: "two"}, "1": {Kind: "one"}}}
	if a.RelationCodesHash() != b.RelationCodesHash() {
		t.Fatal("hash depends on map order")
	}
}
