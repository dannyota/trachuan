package retrieval

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func cursorEngine() *Engine {
	return &Engine{release: Release{Rel: "vn-20990101"}, options: Options{Now: func() time.Time { return time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC) }}}
}

func TestCursor_BoundToRequestAndRelease(t *testing.T) {
	e := cursorEngine()
	req := Request{Query: "Nội dung", Limit: 5, DocTypes: []string{" Café ", "Law"}}
	normalized, c, err := e.searchRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	c.TakenPrimary = 5
	token := encodeSearchCursor(c)
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), req.Query) || strings.Contains(string(raw), "Café") {
		t.Fatal("cursor contains request text")
	}
	for _, limit := range []int{5, 8, 50} {
		same := req
		same.Cursor = token
		same.Limit = limit
		same.DocTypes = []string{"Law", "Cafe\u0301"}
		_, got, err := e.searchRequest(same)
		if err != nil || got != c {
			t.Fatalf("limit %d: %+v %v", limit, got, err)
		}
	}
	for _, change := range []func(*Request){func(r *Request) { r.Query = "Khác" }, func(r *Request) { r.DocTypes = []string{"Other"} }, func(r *Request) { r.Nodes = []string{"1"} }, func(r *Request) { r.Documents = []string{"vn:1"} }, func(r *Request) { r.Issuers = []string{"Other"} }, func(r *Request) { v := false; r.InForceOnly = &v }, func(r *Request) { r.AsOf = &Date{2098, time.January, 1} }, func(r *Request) { r.Mode = "exact" }, func(r *Request) { r.Arms = "dense" }} {
		other := normalized
		other.Cursor = token
		change(&other)
		_, _, err := e.searchRequest(other)
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatal("request mismatch accepted", err)
		}
	}
	e.release.Rel = "vn-20990102"
	req.Cursor = token
	if _, _, err := e.searchRequest(req); !errors.Is(err, ErrReleaseExpired) {
		t.Fatal(err)
	}
}

func TestCursor_KeepsAsOfAcrossMidnight(t *testing.T) {
	e := cursorEngine()
	req, c, err := e.searchRequest(Request{Query: "Nội dung", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	req.Cursor = encodeSearchCursor(c)
	e.options.Now = func() time.Time { return time.Date(2099, 1, 2, 17, 0, 0, 0, time.UTC) }
	next, got, err := e.searchRequest(req)
	if err != nil || got.AsOf.String() != "2099-01-01" || e.searchDate(next) != "2099-01-01" || next.AsOf != nil {
		t.Fatal(next, got, err)
	}
}

func TestCursor_Malformed(t *testing.T) {
	e := cursorEngine()
	_, valid, err := e.searchRequest(Request{Query: "Nội dung"})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"!", strings.Repeat("a", 2049), base64.RawURLEncoding.EncodeToString([]byte(`{}`)), base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"as_of":"2099-13-01"}`))} {
		if _, err := decodeSearchCursor(token); !errors.Is(err, ErrInvalidRequest) {
			t.Fatal("accepted malformed cursor")
		}
	}
	for _, change := range []func(*searchCursor){func(c *searchCursor) { c.V = 2 }, func(c *searchCursor) { c.FirstLimit = 51 }, func(c *searchCursor) { c.TakenPrimary = -1 }, func(c *searchCursor) { c.TakenNonCurrent = 151 }, func(c *searchCursor) { c.Digest = "bad" }, func(c *searchCursor) { c.Release = "" }} {
		c := valid
		change(&c)
		if _, err := decodeSearchCursor(encodeSearchCursor(c)); !errors.Is(err, ErrInvalidRequest) {
			t.Fatal("accepted invalid fields", c)
		}
	}
	raw, _ := base64.RawURLEncoding.DecodeString(encodeSearchCursor(valid))
	for _, raw := range []string{string(raw) + ` {}`, strings.TrimSuffix(string(raw), "}") + `,"query":"secret"}`} {
		if _, err := decodeSearchCursor(base64.RawURLEncoding.EncodeToString([]byte(raw))); !errors.Is(err, ErrInvalidRequest) {
			t.Fatal("accepted extra data")
		}
	}
}

func TestCursor_RequiresEveryField(t *testing.T) {
	e := cursorEngine()
	_, valid, err := e.searchRequest(Request{Query: "Nội dung"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(valid)
	for _, field := range []string{"v", "release", "first_limit", "taken_primary", "taken_non_current", "as_of", "digest"} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		delete(fields, field)
		missing, _ := json.Marshal(fields)
		if _, err := decodeSearchCursor(base64.RawURLEncoding.EncodeToString(missing)); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("accepted missing %s", field)
		}
		fields[strings.ToUpper(field)] = json.RawMessage(`0`)
		wrongCase, _ := json.Marshal(fields)
		if _, err := decodeSearchCursor(base64.RawURLEncoding.EncodeToString(wrongCase)); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("accepted renamed %s", field)
		}
	}
}

func TestCursor_PagingSizes(t *testing.T) {
	a, r := pagingFixture(10, 90)
	e := cursorEngine()
	c := searchCursor{}
	seen := map[Identity]bool{}
	for page := 0; ; page++ {
		limits := []int{5, 8, 50}
		req := Request{Query: "Nội dung", Limit: limits[page%3], Arms: "dense"}
		if page > 0 {
			req.Cursor = encodeSearchCursor(c)
		}
		req, candidate, err := e.searchRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		c = candidate
		out := walkPage(t, a, r, req, c)
		for _, id := range pageIdentities(out) {
			if seen[id] {
				t.Fatal("overlap")
			}
			seen[id] = true
		}
		if out.NextCursor == "" {
			break
		}
		c, err = decodeSearchCursor(out.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 100 {
		t.Fatal("missing hits", len(seen))
	}
}
