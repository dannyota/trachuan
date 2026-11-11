package build

import (
	"encoding/json"
	"os"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func TestValidity_Dieu37Acceptance(t *testing.T) {
	b, err := os.ReadFile("../../validity/testdata/52-2024-dieu-37.txt")
	if err != nil {
		t.Fatal(err)
	}
	reading := validity.Parse("Nghị định", string(b))
	docs := []validityDocument{
		{key: "52", number: "52/2024/NĐ-CP", docType: "Nghị định", issuer: "Chính phủ", issue: "2024-05-15", statusCode: "CCHL", sourceClass: "not_yet", from: "2027-07-01", rawFrom: "2027-07-01", article: &reading},
		{key: "101", number: "101/2012/NĐ-CP", docType: "Nghị định", issuer: "Chính phủ", issue: "2012-11-22", sourceClass: "in_force", from: "2013-01-01", to: "2027-07-01"},
		{key: "80", number: "80/2016/NĐ-CP", docType: "Nghị định", issuer: "Chính phủ", issue: "2016-07-01", sourceClass: "in_force", from: "2016-07-01"},
		{key: "16", number: "16/2019/NĐ-CP", docType: "Nghị định", issuer: "Chính phủ", issue: "2019-02-01", sourceClass: "in_force", from: "2019-02-01"},
	}
	got, err := runValidity(docs, nil, nil, "2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	if got.documents["52"].values.Class != "in_force" || got.documents["52"].values.From == nil || *got.documents["52"].values.From != "2024-07-01" {
		t.Fatalf("actor: %+v", got.documents["52"])
	}
	for _, key := range []string{"101", "80"} {
		d := got.documents[key].values
		if d.To == nil || *d.To != "2024-07-01" || d.Class != "in_force" {
			t.Errorf("%s: %+v", key, d)
		}
	}
	if got.documents["16"].values.To != nil {
		t.Fatal("partial repeal ended document")
	}
}

func TestDecisions_ValuesUseSchemaKeys(t *testing.T) {
	b, err := json.Marshal(validityValues{Class: "in_force", Current: true, Basis: "source_status"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"status_class":"in_force","is_current":true,"status_basis":"source_status","eff_from":null,"eff_to":null,"validity_conflict":false}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
}

func TestDecisions_ManualEvidenceKeyRecordedAsActor(t *testing.T) {
	for _, tc := range []struct {
		name, evidenceKey, actor, artifact string
	}{
		{"other document", "b", "b", "evidence-hash"},
		{"same document default", "", "", "target-hash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
			a.extractHash = "target-hash"
			b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
			b.extractHash = "evidence-hash"
			eligible := false
			correction := source.Correction{Eligible: &eligible, Evidence: source.CorrectionEvidence{Key: tc.evidenceKey, Anchor: "dieu-37/khoan-1", Quote: "evidence"}}
			v := runTest(t, []validityDocument{a, b}, nil, map[string]source.Correction{"a": correction})
			history := v.documents["a"].history
			if len(history) != 2 {
				t.Fatalf("history = %+v", history)
			}
			decision := history[1]
			if decision.layer != "manual" || decision.actor != tc.actor || decision.artifact != tc.artifact || decision.anchor != "dieu-37/khoan-1" || decision.quote != "evidence" {
				t.Fatalf("manual decision = %+v", decision)
			}
		})
	}
}
