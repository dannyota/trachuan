package evidence

import (
	"context"
	"testing"
)

func TestPartialWithoutProvisionEffect(t *testing.T) {
	rel := &fakeRelease{documents: []releaseDocument{
		{ID: 1, Key: "vn:1", StatusClass: "partial"},
		{ID: 2, Key: "vn:2", StatusClass: "partial", ProvisionTargeted: true},
		{ID: 3, Key: "vn:3", StatusClass: "in_force"},
		{ID: 4, Key: "vn:4", StatusClass: "partial", Baseline: true},
	}}
	obs, err := detectPartial(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, f := range obs.Findings {
		keys = append(keys, f.Subject.Fields[0])
	}
	if len(keys) != 2 || keys[0] != "vn:1" || keys[1] != "vn:4" {
		t.Fatalf("findings %v", keys)
	}
	if obs.Observed != 3 || obs.NotObservable != 1 || obs.ObservedCompletely(docSubject("vn:4")) || !obs.ObservedCompletely(docSubject("vn:3")) {
		t.Fatalf("%+v", obs)
	}
}
