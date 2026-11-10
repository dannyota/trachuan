//go:build integration

package retrieval

import (
	"context"
	"testing"
)

func TestPresent_LoadsDocumentsForAllHits(t *testing.T) {
	_, e, _ := fixture(t)
	a, b := presentationHit("vn:1", "p11"), presentationHit("vn:2", "p21")
	a.Stamp = Validity{StatusClass: "in_force", Current: true}
	b.Stamp = Validity{StatusClass: "expired"}
	got, err := e.Present(context.Background(), Response{Hits: []Hit{a, a}, NonCurrent: []Hit{b}}, DetailCompact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 2 || got.Documents["vn:1"].Title != "title" || !got.Documents["vn:1"].Status.Current || got.Documents["vn:2"].Status.Class != "expired" || len(got.Hits) != 2 || len(got.NonCurrent) != 1 {
		t.Fatalf("result: %+v", got)
	}
}
