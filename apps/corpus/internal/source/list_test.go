package source

import (
	"os"
	"reflect"
	"strings"
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
	wantVBPL := []string{"71/2025/QH15", "368/2025/NĐ-CP", "58/2021/NĐ-CP", "18/2018/TT-NHNN", "134/2025/QH15", "53/2026/TT-NHNN", "142/2026/NĐ-CP", "88/2019/NĐ-CP", "143/2021/NĐ-CP", "340/2025/NĐ-CP", "52/2024/NĐ-CP", "304/2025/NĐ-CP"}
	wantChinhphu := ChinhphuSources{IssuerPolicyVersion: 1, CentralIssuerForms: []string{"Chính phủ", "Thủ tướng Chính phủ", "Quốc hội", "Bộ Khoa học và Công nghệ", "Ngân hàng Nhà nước Việt Nam"}, Keywords: []string{"trí tuệ nhân tạo"}, NamedNumbers: []string{"142/2026/NĐ-CP", "33/2026/QĐ-TTg", "05/2026/TT-BKHCN", "134/2025/QH15"}}
	if list.VBPL.SelectionVersion != 1 || !reflect.DeepEqual(list.VBPL.NamedNumbers, wantVBPL) || !reflect.DeepEqual(list.Chinhphu, wantChinhphu) {
		t.Fatalf("unexpected named rules: %+v %+v", list.VBPL, list.Chinhphu)
	}
	if list.Country != "vn" || len(list.VBPL.SweepAgencies) != 2 || len(list.VBPL.KeywordAgencies) != 8 || len(list.VBPL.Keywords) != 32 || len(list.RelationCodes) != 11 || len(list.StatusCodes) != 8 {
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

func TestParse_YearlessNamedNeedsDate(t *testing.T) {
	for _, tc := range []struct {
		entry string
		valid bool
	}{
		{"367/QĐ-TTg", false}, {"367/QĐ-TTg@2026-02-28", true},
		{"1671/QĐ-TTg", false}, {"1671/QĐ-TTg@2026-08-28", true},
		{"18/2018/TT-NHNN", true}, {"123/2026", false},
		{"367/QĐ-TTg@2026-02-30", false}, {"71/2025/QH15@bad", false},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			_, err := Parse([]byte("vbpl:\n  named_numbers: [\"" + tc.entry + "\"]\n"))
			if (err == nil) != tc.valid {
				t.Fatalf("entry %q: error = %v", tc.entry, err)
			}
			if err != nil && !strings.Contains(err.Error(), tc.entry) {
				t.Fatalf("error does not name entry: %v", err)
			}
		})
	}
}

func TestParse_NamedNumbersAndChinhphu(t *testing.T) {
	list, err := Parse([]byte("vbpl:\n  selection_version: 1\n  named_numbers: ['367/QĐ-TTg@2026-02-28']\nchinhphu:\n  issuer_policy_version: 1\n  central_issuer_forms: ['Chính phủ']\n  keywords: ['trí tuệ nhân tạo']\n  named_numbers: ['71/2025/QH15']\n"))
	if err != nil {
		t.Fatal(err)
	}
	if list.VBPL.SelectionVersion != 1 || len(list.VBPL.NamedNumbers) != 1 || len(list.Chinhphu.Keywords) != 1 || len(list.Chinhphu.NamedNumbers) != 1 {
		t.Fatalf("unexpected list: %+v", list)
	}
	entry, err := ParseNamedNumber(list.VBPL.NamedNumbers[0])
	if err != nil || entry.Number != "367/QĐ-TTg" || entry.Date != "2026-02-28" {
		t.Fatalf("entry = %+v, error = %v", entry, err)
	}
}

func TestParse_ChinhphuIssuerPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, policy string
		valid        bool
	}{
		{"missing forms", "issuer_policy_version: 1", false},
		{"empty form", "issuer_policy_version: 1\n  central_issuer_forms: [' ']", false},
		{"duplicate normalized forms", "issuer_policy_version: 1\n  central_issuer_forms: ['Chính phủ', ' CHÍNH  PHỦ ']", false},
		{"unsupported version", "issuer_policy_version: 2\n  central_issuer_forms: ['Chính phủ']", false},
		{"valid", "issuer_policy_version: 1\n  central_issuer_forms: ['Chính phủ']", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			list, err := Parse([]byte("chinhphu:\n  " + tc.policy + "\n"))
			if err == nil {
				err = list.ValidateSecondary()
			}
			if (err == nil) != tc.valid {
				t.Fatalf("error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

func TestParse_HistoricalChinhphuSnapshot(t *testing.T) {
	_, err := Parse([]byte(`{"vbpl":{"selection_version":1},"chinhphu":{"keywords":["law"]}}`))
	if err != nil {
		t.Fatalf("historical snapshot rejected: %v", err)
	}
}
