//go:build integration

package build

import (
	"context"
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestLoad_ClassAndSourceFromSnapshot(t *testing.T) {
	f := fixture(t)
	key := "vn:vbpl:1"
	f.fetch(t, key)
	inv := f.inventory(t)
	inv.Version = 2
	inv.Membership.Entries = []membership.Entry{{DocumentKey: key, Disposition: "active", Class: "relation_context", Source: "vbpl"}}
	if _, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf()); err != nil {
		t.Fatal(err)
	}
	var class, source string
	if err := f.corpus.QueryRow(context.Background(), `SELECT class,source FROM document WHERE key=$1`, key).Scan(&class, &source); err != nil {
		t.Fatal(err)
	}
	if class != "relation_context" || source != "vbpl" {
		t.Fatalf("class=%s source=%s", class, source)
	}
}

func TestLoad_EachExcludedDispositionNotLoaded(t *testing.T) {
	f := fixture(t)
	dispositions := []string{"withdrawn", "scope_removed", "source_replaced", "fetch gap"}
	for n := range dispositions {
		f.fetch(t, fmt.Sprintf("vn:vbpl:%d", n+1))
	}
	inv := f.inventory(t)
	inv.Version = 2
	for n, disposition := range dispositions {
		key := fmt.Sprintf("vn:vbpl:%d", n+1)
		entry := membership.Entry{DocumentKey: key, Disposition: disposition, Class: "primary", Source: "vbpl"}
		if disposition == "fetch gap" {
			entry.Disposition = "active"
			for i := range inv.Items {
				if inv.Items[i].Stage == "fetch" && inv.Items[i].ItemKey == key {
					inv.Items[i].State = "gap"
				}
			}
		}
		inv.Membership.Entries = append(inv.Membership.Entries, entry)
	}
	counts, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	if counts.Documents != 0 {
		t.Fatalf("documents=%d", counts.Documents)
	}
	for n, disposition := range dispositions[:3] {
		key := fmt.Sprintf("vn:vbpl:%d", n+1)
		var count int
		if err = f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM coverage_gap WHERE document_key=$1 AND kind=$2`, key, disposition).Scan(&count); err != nil || count != 1 {
			t.Errorf("%s gaps=%d: %v", disposition, count, err)
		}
	}
}
