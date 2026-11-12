package retrieval

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFilterDocuments_NormalizesFacets(t *testing.T) {
	rows := &cacheRows{values: [][]any{
		{"vn:1", " Cafe\u0301 ", " Co\u031b quan "},
		{"vn:2", " law ", " other "},
	}}
	docs, err := queryFilterDocuments(t.Context(), rowsTx{rows: rows}, []string{"vn:1", "vn:2", "vn:1"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]DocumentEntry{
		"vn:1": {Type: "Café", Issuer: "Cơ quan"},
		"vn:2": {Type: "law", Issuer: "other"},
	}
	if !reflect.DeepEqual(docs, want) {
		t.Fatal("filter metadata changed", docs)
	}
	for key, inside := range map[string]bool{"vn:1": true, "vn:2": false} {
		req := Request{DocTypes: []string{"Café"}, Issuers: []string{"Cơ quan"}}
		if got := matchesFilter(req, Identity{DocumentKey: key}, nil, docs[key]); got != inside {
			t.Fatalf("%s: in_filter=%t", key, got)
		}
	}
}

func TestFilterDocuments_EmptyKeysSkipDatabase(t *testing.T) {
	docs, err := queryFilterDocuments(t.Context(), nil, nil)
	if err != nil || len(docs) != 0 {
		t.Fatal(docs, err)
	}
	if _, err := (&Engine{}).filterDocuments(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestFilterDocuments_ErrorsAreSanitized(t *testing.T) {
	cause := errors.New("private database details")
	for name, db := range map[string]filterDocumentReader{
		"query": failingTx{err: cause},
		"scan":  rowsTx{rows: &failingRows{scan: true, err: cause}},
		"rows":  rowsTx{rows: &failingRows{err: cause}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := queryFilterDocuments(t.Context(), db, []string{"vn:1"})
			if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe or unclassified filter failure", err)
			}
		})
	}
}
