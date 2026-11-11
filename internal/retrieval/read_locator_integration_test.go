//go:build integration

package retrieval

import (
	"testing"
)

func TestRead_LocatorIntegration(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `UPDATE document SET text_source='html_transcription' WHERE id=1`)
	readExec(t, db, `UPDATE provision SET locator='block:2-4' WHERE id=9`)
	for _, item := range []ReadItem{{Handle: "vn:1"}, {Handle: "vn:1", From: "p9", To: "p9"}, {Handle: "vn:1#p9"}} {
		pages := 0
		found := false
		for {
			r := readItems(t, e, item).Items[0]
			if r.Error != "" {
				t.Fatal(r.Error)
			}
			for _, u := range r.Units {
				if u.Anchor == "p9" {
					found = true
					if u.Locator == nil || *u.Locator != "block:2-4" {
						t.Fatalf("locator: %+v", u)
					}
				}
			}
			pages++
			if r.Next == "" {
				break
			}
			if pages > 10 {
				t.Fatal("too many pages")
			}
			item = ReadItem{Next: r.Next}
		}
		if !found {
			t.Fatal("missing target unit")
		}
	}
}
