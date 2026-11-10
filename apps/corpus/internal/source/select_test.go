package source

import (
	"reflect"
	"testing"
)

func TestSelect_AgencyAndTermSupports(t *testing.T) {
	got, err := Select([]ListingItem{{ID: "1", Title: "Ngân hàng và dữ liệu cá nhân", AgencyIDs: []string{"62"}}}, VBPLSources{SelectionVersion: 1, SweepAgencies: []string{"62"}, Keywords: []string{"ngân hàng", "dữ liệu cá nhân"}})
	want := []SelectedItem{{ID: "1", Rules: []SelectionRule{{Kind: "agency", Value: "62"}, {Kind: "term", Value: "dữ liệu cá nhân"}, {Kind: "term", Value: "ngân hàng"}}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, error %v", got, err)
	}
}

func TestSelect_ContiguousNFCPhraseNoFolding(t *testing.T) {
	items := []ListingItem{{ID: "1", Title: "Nghị định số 52/2024/NĐ-CP quy định về thanh toán không dung tiền mặt"}, {ID: "2", Title: "THANH TOA\u0301N\u00a0 không   dùng tiền mặt"}, {ID: "3", Title: "thanh toán không dùng tiền mặt"}}
	got, err := Select(items, VBPLSources{SelectionVersion: 1, Keywords: []string{"thanh toán không dùng tiền mặt"}})
	if err != nil || len(got) != 2 || got[0].ID != "2" || got[1].ID != "3" {
		t.Fatalf("got %+v, error %v", got, err)
	}
}

func TestSelect_ExcludesRenditions(t *testing.T) {
	got, err := Select([]ListingItem{{ID: "vbpqta_123", Title: "ngân hàng", AgencyIDs: []string{"62"}}}, VBPLSources{SelectionVersion: 1, SweepAgencies: []string{"62"}, Keywords: []string{"ngân hàng"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, error %v", got, err)
	}
}
