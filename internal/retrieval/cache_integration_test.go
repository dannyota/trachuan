//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"reflect"
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
