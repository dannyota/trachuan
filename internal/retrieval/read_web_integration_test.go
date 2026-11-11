//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestDocuments_WholeDocumentMatchesRange(t *testing.T) {
	_, e := readFixture(t)
	req := ReadRequest{Release: e.release.Rel, WholeDocument: true, DocumentKey: "vn:1", PageCap: 2_000_000, PartSize: 1_000_000, Items: []ReadItem{{Handle: "vn:1"}}}
	got, err := e.Read(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	page := got.Items[0]
	if page.Document == nil || !page.Document.Status.Current || !page.Complete || page.Next != "" {
		t.Fatal("missing document or text")
	}
	rangeReq := req
	rangeReq.WholeDocument = false
	rangeReq.DocumentKey = ""
	rangeReq.Items = []ReadItem{{Handle: "vn:1", From: "p1", To: "p9"}}
	text, err := e.Read(context.Background(), rangeReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Units) != 5+len(text.Items[0].Units) || !reflect.DeepEqual(page.Units[5:], text.Items[0].Units) {
		t.Fatal("whole document changed range text")
	}
	for _, u := range page.Units[:5] {
		if u.Kind != "outline" {
			t.Fatal("text before outline")
		}
	}
}

func TestDocuments_WholeDocumentPaginationAndTokenIsolation(t *testing.T) {
	db, e := readFixture(t)
	source := strings.Repeat("đ", 1_200_000)
	readExec(t, db, `UPDATE provision SET text=$1 WHERE id=9`, source)
	req := ReadRequest{Release: e.release.Rel, WholeDocument: true, DocumentKey: "vn:1", PageCap: 2_000_000, PartSize: 1_000_000, Items: []ReadItem{{Handle: "vn:1"}}}
	first, err := e.Read(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Items[0].Next == "" {
		t.Fatal("missing web continuation")
	}
	var joined strings.Builder
	page := first.Items[0]
	for i := 0; i < 10; i++ {
		for _, u := range page.Units {
			if u.Anchor == "p9" && u.Kind == "provision" {
				joined.WriteString(u.Text)
			}
		}
		if page.Next == "" {
			if !page.Complete {
				t.Fatal("incomplete chain")
			}
			break
		}
		prior := page.Next
		req.Items = []ReadItem{{Next: prior}}
		next, readErr := e.Read(context.Background(), req)
		if readErr != nil {
			t.Fatal(readErr)
		}
		page = next.Items[0]
		if page.Error != "" || page.Next == prior || len(page.Units) == 0 {
			t.Fatal("no progress")
		}
	}
	if joined.String() != source {
		t.Fatal("document parts changed source")
	}
	mcp := readItems(t, e, ReadItem{Next: first.Items[0].Next})
	if mcp.Items[0].Error != "invalid_next" {
		t.Fatal("MCP accepted web token")
	}
	mcp = readItems(t, e, ReadItem{Handle: "vn:1#p9"})
	if mcp.Items[0].Next == "" {
		t.Fatal("missing MCP continuation")
	}
	req.Items = []ReadItem{{Next: mcp.Items[0].Next}}
	result, err := e.Read(context.Background(), req)
	if err != nil || result.Items[0].Error != "invalid_next" {
		t.Fatal("web accepted MCP token")
	}
}
