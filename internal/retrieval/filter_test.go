package retrieval

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFilter_KindsAndValues(t *testing.T) {
	req, err := normalizeFilters(Request{Nodes: []string{" 1.1 ", "1.1"}, DocTypes: []string{" Café ", "Cafe\u0301"}, Issuers: []string{" Cơ quan ", "Other"}, Documents: []string{"vn:1", "vn:2"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req.DocTypes, []string{"Café"}) || len(req.Nodes) != 1 {
		t.Fatal(req)
	}
	id := Identity{DocumentKey: "vn:1"}
	doc := DocumentEntry{Type: " Cafe\u0301 ", Issuer: " Cơ quan "}
	if !matchesFilter(req, id, []string{"1.1.c1"}, doc) {
		t.Fatal("matching hit outside")
	}
	for _, change := range []func(*Request){func(r *Request) { r.Nodes = []string{"1.2"} }, func(r *Request) { r.DocTypes = []string{"law"} }, func(r *Request) { r.Issuers = []string{"issuer"} }, func(r *Request) { r.Documents = []string{"vn:3"} }} {
		other := req
		change(&other)
		if matchesFilter(other, id, []string{"1.1.c1"}, doc) {
			t.Fatal("kinds are not ANDed")
		}
	}
	if matchesFilter(req, id, []string{"1.10.c1"}, doc) {
		t.Fatal("node prefix lacked boundary")
	}
}

func TestFilter_Budgets(t *testing.T) {
	for _, tc := range []struct {
		max int
		set func(*Request, []string)
	}{{20, func(r *Request, v []string) { r.Nodes = v }}, {10, func(r *Request, v []string) { r.DocTypes = v }}, {10, func(r *Request, v []string) { r.Issuers = v }}, {20, func(r *Request, v []string) { r.Documents = v }}} {
		for _, n := range []int{tc.max, tc.max + 1} {
			v := make([]string, n)
			for i := range v {
				v[i] = "1"
			}
			var r Request
			tc.set(&r, v)
			_, err := normalizeFilters(r)
			if (n > tc.max) != errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("size %d: %v", n, err)
			}
		}
		for _, value := range []string{"", " \t ", "bad\x00", string([]byte{255})} {
			var r Request
			tc.set(&r, []string{value})
			if _, err := normalizeFilters(r); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("accepted invalid value %q", value)
			}
		}
	}
	for _, code := range []string{"0", "1..2", "1.1.z1", strings.Repeat("1", 300)} {
		if _, err := normalizeFilters(Request{Nodes: []string{code}}); !errors.Is(err, ErrInvalidRequest) {
			t.Fatal(code, err)
		}
	}
}

func TestFilter_PermutationOfUnfiltered(t *testing.T) {
	a, r := pagingFixture(18, 22)
	w, err := buildSearchWalk(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Limit: 8, Arms: "dense", Documents: []string{"vn:001", "vn:008", "vn:019", "vn:038"}}
	want := [][]Identity{}
	for _, gs := range [][]Group{w.primary, w.nonCurrent} {
		var ids []Identity
		for _, inside := range []bool{true, false} {
			for _, g := range gs {
				if matchesFilter(req, g.Identity, nil, DocumentEntry{}) == inside {
					ids = append(ids, g.Identity)
				}
			}
		}
		want = append(want, ids)
	}
	var got [2][]Identity
	c := searchCursor{FirstLimit: 8}
	for page := 0; ; page++ {
		out := walkPage(t, a, r, req, c)
		if page == 0 && (len(out.Hits) != 6 || len(out.NonCurrent) != 2) {
			t.Fatal("first group counts changed")
		}
		if out.OutsideFilterCount == nil || *out.OutsideFilterCount != 36 {
			t.Fatal("outside count", out.OutsideFilterCount)
		}
		for i, hs := range [][]Hit{out.Hits, out.NonCurrent} {
			for _, h := range hs {
				if h.InFilter == nil || *h.InFilter != matchesFilter(req, h.Identity, nil, DocumentEntry{}) {
					t.Fatal("in_filter")
				}
				got[i] = append(got[i], h.Identity)
			}
		}
		if out.NextCursor == "" {
			break
		}
		c, err = decodeSearchCursor(out.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := range got {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatal("unstable partition", i)
		}
	}
}

func TestFilter_ZeroInside(t *testing.T) {
	a, r := pagingFixture(4, 4)
	out := walkPage(t, a, r, Request{Limit: 8, Documents: []string{"missing"}}, searchCursor{FirstLimit: 8})
	if len(pageIdentities(out)) != 8 || out.OutsideFilterCount == nil || *out.OutsideFilterCount != 8 {
		t.Fatal(out)
	}
	for _, hs := range [][]Hit{out.Hits, out.NonCurrent} {
		for _, h := range hs {
			if h.InFilter == nil || *h.InFilter {
				t.Fatal("outside hit")
			}
		}
	}
}

func TestFilter_AnyKindSetsInFilter(t *testing.T) {
	a, r := pagingFixture(1, 1)
	for _, req := range []Request{{DocTypes: []string{"law"}}, {Issuers: []string{"issuer"}}, {Documents: []string{"vn:000"}}} {
		req.Limit = 8
		out := walkPage(t, a, r, req, searchCursor{FirstLimit: 8})
		for _, hs := range [][]Hit{out.Hits, out.NonCurrent} {
			for _, h := range hs {
				if h.InFilter == nil {
					t.Fatal("missing in_filter")
				}
			}
		}
	}
}

func TestFilter_ValidityGroups(t *testing.T) {
	a, r := pagingFixture(2, 2)
	a.IncludeNonCurrent = true
	out := walkPage(t, a, r, Request{Limit: 8, Documents: []string{"vn:001", "vn:003"}}, searchCursor{FirstLimit: 8})
	if len(out.Hits) != 2 || len(out.NonCurrent) != 2 || *out.OutsideFilterCount != 2 {
		t.Fatal(out)
	}
	for _, hits := range [][]Hit{out.Hits, out.NonCurrent} {
		if hits[0].InFilter == nil || !*hits[0].InFilter || hits[1].InFilter == nil || *hits[1].InFilter {
			t.Fatal("group not ordered inside first", hits)
		}
	}
	if !out.Hits[1].Stamp.Current || out.NonCurrent[0].Stamp.Current {
		t.Fatal("validity group changed")
	}
	want := []Identity{a.Primary.Dense[1].Group, a.Primary.Dense[0].Group, a.NonCurrent.Dense[1].Group, a.NonCurrent.Dense[0].Group}
	if !reflect.DeepEqual(pageIdentities(out), want) {
		t.Fatal("primary must precede non-current")
	}
}
