package vbpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
)

func TestLookupNumber_UniqueCollisionEmpty(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
	}{{"lookup-71-2025-QH15.json", 1}, {"lookup-367-QD-TTg.json", 3}, {"lookup-142-2026-ND-CP.json", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fixture(t, tc.name)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q map[string]any
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q["optionDoc"] != "docNum" || q["matchMode"] != "exact" || q["agencyLevel"] != "TRUNG_UONG" || q["keyword"] != "number" || q["agencyIds"] != nil {
					t.Errorf("query = %v, err = %v", q, err)
				}
				_, _ = w.Write(raw)
			}))
			defer s.Close()
			result, pages, err := testClient(s).LookupNumber(context.Background(), "number")
			if err != nil || len(result.Items) != tc.count || result.Evidence.Completeness != lookup.Complete || len(pages) != 1 {
				t.Fatalf("result = %+v, pages = %d, err = %v", result, len(pages), err)
			}
			sum := sha256.Sum256(raw)
			if result.Evidence.PagesFetched[0].SHA256 != hex.EncodeToString(sum[:]) || string(pages[0]) != string(raw) {
				t.Fatal("raw evidence differs")
			}
			if tc.count > 0 && result.Items[0].PageRef != result.Evidence.PagesFetched[0].SHA256 {
				t.Fatalf("item page reference = %q", result.Items[0].PageRef)
			}
			if tc.count == 0 {
				result.Evidence.PagesFetched[0].Path = "page.json"
				if !result.ProvesAbsence() {
					t.Fatal("empty result does not prove absence")
				}
			}
		})
	}
}

func TestLookupNumber_PageFailureIsIncomplete(t *testing.T) {
	raw := fixture(t, "lookup-367-QD-TTg-size2-page1.json")
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write(raw)
		} else {
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	c := testClient(s)
	c.lookupPageSize = 2
	result, pages, err := c.LookupNumber(context.Background(), "367/QĐ-TTg")
	if err == nil || result.Evidence.Completeness != lookup.Incomplete || len(result.Items) != 2 || len(pages) != 1 {
		t.Fatalf("result = %+v pages = %d err = %v", result, len(pages), err)
	}
}

func TestList_CentralNoAgencyNoKeyword(t *testing.T) {
	raw := fixture(t, "list-central-size5-page1.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q["pageSize"] != float64(1000) || q["agencyIds"] != nil || q["keyword"] != nil {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write(raw)
	}))
	defer s.Close()
	p, err := testClient(s).List(context.Background(), ListQuery{Page: 1, PageSize: 1000})
	if err != nil || len(p.Items) != 5 || len(p.Items[0].AgencyIDs) == 0 {
		t.Fatalf("page = %+v, err = %v", p, err)
	}
}

func TestLookupNumber_PagedTotalsAndEmptyLastPage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pages    []string
		count    int
		complete lookup.Completeness
	}{
		{"all", []string{"lookup-367-QD-TTg-size2-page1.json", "lookup-367-QD-TTg-size2-page2.json"}, 3, lookup.Complete},
		{"empty early", []string{"lookup-367-QD-TTg-size2-page1.json", "lookup-367-QD-TTg-size2-page3.json"}, 2, lookup.Incomplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies [][]byte
			for _, name := range tc.pages {
				bodies = append(bodies, fixture(t, name))
			}
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var query struct {
					Page int `json:"pageNumber"`
					Size int `json:"pageSize"`
				}
				if err := json.NewDecoder(r.Body).Decode(&query); err != nil || query.Page != calls+1 || query.Size != 2 {
					t.Errorf("query = %+v, error = %v", query, err)
				}
				if calls < len(bodies) {
					_, _ = w.Write(bodies[calls])
				}
				calls++
			}))
			defer s.Close()
			c := testClient(s)
			c.lookupPageSize = 2
			result, pages, err := c.LookupNumber(context.Background(), "367/QĐ-TTg")
			if err != nil || calls != 2 || len(result.Items) != tc.count || len(pages) != 2 || result.Evidence.Completeness != tc.complete {
				t.Fatalf("result = %+v, pages = %d, calls = %d, error = %v", result, len(pages), calls, err)
			}
		})
	}
}

func TestList_ZeroValuesUnchanged(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		if q["pageSize"] != float64(50) || q["optionDoc"] != "title" || q["matchMode"] != "all_words" || q["agencyLevel"] != "TRUNG_UONG" || q["agencyIds"] != nil || q["keyword"] != nil {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"total":0,"items":[]}}`))
	}))
	defer s.Close()
	if _, err := testClient(s).List(context.Background(), ListQuery{}); err != nil {
		t.Fatal(err)
	}
}
