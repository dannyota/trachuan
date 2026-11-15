//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadCache_EngineChains(t *testing.T) {
	_, e := readFixture(t)
	cache := &memoryCache{}
	items := []ReadItem{{Handle: "vn:1"}, {Handle: "vn:1#p9"}, {Handle: "vn:1", From: "p4", To: "p9"}, {Number: "N2"}, {Handle: "vn:1", Relations: true}}
	for _, item := range items {
		collect := func(cached bool) []string {
			t.Helper()
			e.options.Sequences = nil
			if cached {
				e.options.Sequences = cache
			}
			current := item
			var pages []string
			for page := 0; page < 20; page++ {
				result := readItems(t, e, current)
				raw, _ := json.Marshal(result)
				pages = append(pages, string(raw))
				if result.Items[0].Next == "" {
					return pages
				}
				current = ReadItem{Next: result.Items[0].Next}
			}
			t.Fatal("chain did not finish")
			return nil
		}
		cold := collect(false)
		warm, hit := collect(true), collect(true)
		if !reflect.DeepEqual(cold, warm) || !reflect.DeepEqual(cold, hit) {
			t.Fatal("engine cache changed pages")
		}
	}
	if cache.hits == 0 || cache.puts == 0 {
		t.Fatal("engine did not use sequence hook")
	}
}

func TestDocumentCache_EngineMetadataAndReadStatus(t *testing.T) {
	db, e := readFixture(t)
	cache := &memoryCache{}
	cold, err := e.Documents(context.Background(), []string{"vn:1"})
	if err != nil {
		t.Fatal(err)
	}
	e.options.Documents = cache
	warm, err := e.Documents(context.Background(), []string{"vn:1"})
	if err != nil {
		t.Fatal(err)
	}
	hit, err := e.Documents(context.Background(), []string{"vn:1"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cold, warm) || !reflect.DeepEqual(cold, hit) || cache.hits != 1 {
		t.Fatal("metadata changed")
	}
	readExec(t, db, `UPDATE document SET eff_to='2099-01-02' WHERE key='vn:1'`)
	now := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	e.options.Now = func() time.Time { return now }
	e.options.Sequences = &memoryCache{}
	first := readItems(t, e, ReadItem{Handle: "vn:1"}).Items[0]
	now = now.Add(time.Second)
	second := readItems(t, e, ReadItem{Handle: "vn:1"}).Items[0]
	if !first.Document.Status.Current || second.Document.Status.Current {
		t.Fatal("read status survived midnight")
	}
}

func TestCollect_InstantPinsEligibilityAcrossMidnight(t *testing.T) {
	db, e, _ := fixture(t)
	readExec(t, db, `UPDATE document SET eff_to='2099-01-02' WHERE id=1`)
	before := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	clockCalls := 0
	e.options.Now = func() time.Time { clockCalls++; return before.Add(time.Second) }
	p, err := e.Prepare(context.Background(), Request{Query: "ngân hàng", Mode: "exact", Instant: before})
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := e.Collect(context.Background(), p, Depths{Dense: 8, Lexical: 8})
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Date.String() != "2099-01-01" || clockCalls != 0 {
		t.Fatal("Collect resampled the request date")
	}
	found := false
	for _, candidate := range pinned.Primary.Dense {
		found = found || candidate.Identity.DocumentKey == "vn:1"
	}
	if !found {
		t.Fatal("document effective on the pinned date disappeared")
	}
	p.Request.Instant = time.Time{}
	current, err := e.Collect(context.Background(), p, Depths{Dense: 8, Lexical: 8})
	if err != nil {
		t.Fatal(err)
	}
	if current.Date.String() != "2099-01-02" || clockCalls != 1 {
		t.Fatal("zero instant lost clock fallback")
	}
	for _, candidate := range current.Primary.Dense {
		if candidate.Identity.DocumentKey == "vn:1" {
			t.Fatal("expired document remained eligible")
		}
	}
}

func TestReadCache_WhitespaceMetadataKeepsPageBoundaries(t *testing.T) {
	db, e := readFixture(t)
	raw := "\n  Number\u2003  Value  "
	readExec(t, db, `UPDATE document SET number=$1,title=$1 WHERE id=1`, raw)
	readExec(t, db, `INSERT INTO relation(id,source_document_id,target_document_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES (1,2,1,9,'vn:2','vn:1',$1,'N1','repeals','','','p9','evidence','vbpl_references',$2,0)`, raw, strings.Repeat("b", 64))
	cache := &memoryCache{}
	collect := func(mode string) []ReadResult {
		t.Helper()
		if mode == "miss" {
			e.options.Sequences = nil
		} else {
			e.options.Sequences = cache
		}
		item := ReadItem{Handle: "vn:1"}
		var pages []ReadResult
		for i := 0; i < 100; i++ {
			if mode == "evicted" {
				cache.values = nil
			}
			result, err := e.Read(context.Background(), ReadRequest{Release: e.release.Rel, Items: []ReadItem{item}, PageCap: 3000, PartSize: 1500, WholeDocument: true})
			if err != nil || len(result.Items) != 1 || result.Items[0].Error != "" {
				t.Fatalf("%s: %+v %v", mode, result, err)
			}
			pages = append(pages, result)
			if result.Items[0].Next == "" {
				return pages
			}
			item = ReadItem{Next: result.Items[0].Next}
		}
		t.Fatal("chain did not finish")
		return nil
	}
	miss := collect("miss")
	warm := collect("warm")
	hits := cache.hits
	hit := collect("hit")
	if cache.hits <= hits {
		t.Fatal("no cache hit")
	}
	evicted := collect("evicted")
	if len(miss) < 2 || !reflect.DeepEqual(miss, warm) || !reflect.DeepEqual(miss, hit) || !reflect.DeepEqual(miss, evicted) {
		t.Fatal("cache changed page boundaries or output")
	}
	if miss[0].Items[0].Document == nil || miss[0].Items[0].Document.Number != "Number Value" || miss[0].Items[0].Document.Title != "Number Value" {
		t.Fatal("document metadata not normalized")
	}
	found := false
	for _, page := range miss {
		for _, unit := range page.Items[0].Units {
			for _, warning := range unit.Warnings {
				found = true
				if warning.Number != "Number Value" {
					t.Fatalf("warning: %+v", warning)
				}
			}
		}
	}
	if !found {
		t.Fatal("warning missing")
	}
}
