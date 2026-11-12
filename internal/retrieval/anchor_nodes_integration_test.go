//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"testing"
)

func TestAnchorNodes_EffectiveAndUnknown(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	rows, err := e.db.Query(ctx, `SELECT d.key,p.anchor FROM provision p JOIN document d ON d.id=p.document_id WHERE p.id=ANY($1::bigint[]) ORDER BY p.id`, []int64{11, 12})
	if err != nil {
		t.Fatal(err)
	}
	var refs []AnchorRef
	for rows.Next() {
		var ref AnchorRef
		if err = rows.Scan(&ref.DocumentKey, &ref.Anchor); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(refs) != 2 {
		t.Fatalf("refs %d: %v", len(refs), err)
	}
	missing := AnchorRef{DocumentKey: "missing", Anchor: "missing"}
	got, err := e.AnchorNodes(ctx, append(refs, missing))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[refs[0]], []string{"1.1.c1"}) || len(got[refs[1]]) != 0 {
		t.Fatalf("nodes %v", got)
	}
	if _, ok := got[missing]; ok {
		t.Fatal("unknown ref present")
	}
}
