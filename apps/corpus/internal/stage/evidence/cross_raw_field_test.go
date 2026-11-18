package evidence

import (
	"context"
	"testing"
)

func TestRawFieldNotNormalized_FiresPerDocumentAndField(t *testing.T) {
	rel := &fakeRelease{documents: []releaseDocument{
		{ID: 1, Key: "vn:1", Number: "  12/2099/TT-NHNN", Title: "Thông tư"},
		{ID: 2, Key: "vn:2", Number: "13/2099/TT-NHNN", Title: " Title\n"},
		{ID: 3, Key: "vn:3", Number: "14/2099/TT-NHNN", Title: "Café"},
		{ID: 4, Key: "vn:4", Number: "15/2099/TT-NHNN", Title: "Clean", Baseline: true},
	}}
	obs, err := detectRawFields(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range obs.Findings {
		got[f.Subject.Fields[0]] += f.Discriminator + ","
	}
	if len(obs.Findings) != 3 || got["vn:1"] != "number," || got["vn:2"] != "title," || got["vn:3"] != "title," {
		t.Fatalf("findings %+v", obs.Findings)
	}
	if obs.Observed != 3 || obs.NotObservable != 1 {
		t.Fatalf("observed %d not observable %d", obs.Observed, obs.NotObservable)
	}
	if !obs.ObservedCompletely(docSubject("vn:1")) || obs.ObservedCompletely(docSubject("vn:4")) || obs.ObservedCompletely(docSubject("vn:9")) {
		t.Fatal("observed completely is wrong")
	}
}

func TestRawFieldNotNormalized_FingerprintFollowsRawValue(t *testing.T) {
	run := func(number string) Fired {
		obs, err := detectRawFields(context.Background(), &fakeRelease{documents: []releaseDocument{{ID: 1, Key: "vn:1", Number: number, Title: "T"}}})
		if err != nil || len(obs.Findings) != 1 {
			t.Fatalf("%v %+v", err, obs.Findings)
		}
		return obs.Findings[0]
	}
	a, b := run(" 1/2099/TT-NHNN"), run("  1/2099/TT-NHNN")
	fa, _ := hashValue(a.FingerprintInputs)
	fb, _ := hashValue(b.FingerprintInputs)
	if fa == fb {
		t.Fatal("fingerprint ignores the raw value")
	}
}
