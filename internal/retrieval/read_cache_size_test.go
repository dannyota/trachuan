package retrieval

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadCache_SizeEstimateMatchesJSON(t *testing.T) {
	when := time.Date(2099, 5, 1, 12, 0, 1, 120000000, time.FixedZone("Vietnam", 7*60*60))
	date := &Date{Year: 2099, Month: time.May, Day: 1}
	for _, text := range []string{"", "ordinary", "\x00\b\f\n\r\t\"\\<&đ\u2028\u2029", strings.Repeat("đ ", 200), string([]byte{0xff, 0xc0}), "�"} {
		for _, doc := range []*DocumentEntry{nil, {}, {Title: text, Nodes: []string{"1.1.c1", "2.1"}, Issued: date, EffectiveFrom: date, RelatedCounts: map[string]int{"in:implements": 123}, Related: []Pointer{{Dir: "in", InRelease: true}}, Status: Status{Class: "in_force", Current: true}}} {
			seq := readSequence{doc: "vn:1", document: doc, source: Document{EffectiveFrom: &when}, units: []Unit{{Kind: "provision", Text: text, Size: 23}, {Kind: "relation", Relation: &ListedRelation{Pointer: Pointer{Dir: "in", InRelease: true}, EvidenceComplete: true, Evidence: text}}}, parts: map[int][]Unit{0: {{Kind: "provision", Part: 1, Parts: 2, Text: text}}}, gaps: []Gap{{Kind: "gap", Message: text}}, offset: 12, more: true, complete: true}
			stored := cachedSequence{Doc: seq.doc, Source: seq.source, Units: seq.units, Parts: seq.parts, Gaps: seq.gaps, Offset: seq.offset, More: seq.more, Complete: seq.complete}
			if doc != nil {
				stored.Document = metadataJSON(*doc)
			}
			raw, err := json.Marshal(stored)
			if err != nil {
				t.Fatal(err)
			}
			actual := int64(len(raw))
			for _, limit := range []int64{actual, actual + 100} {
				if got := sequenceSize(seq, limit); got != actual {
					t.Fatalf("estimate=%d actual=%d limit=%d", got, actual, limit)
				}
			}
			if got := sequenceSize(seq, actual-1); got <= actual-1 {
				t.Fatal("oversized value underestimated")
			}
		}
	}
}

func TestReadCache_JSONStringSizeMatchesEncoder(t *testing.T) {
	for i := 0; i < 256; i++ {
		text := string([]byte{byte(i)})
		raw, _ := json.Marshal(text)
		if got := cacheJSONStringSize(text, 100); got != int64(len(raw)) {
			t.Fatalf("byte %x: estimate=%d actual=%d JSON=%q", i, got, len(raw), raw)
		}
	}
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	for _, text := range []string{string(all), "\u2028\u2029\ufffdđ", strings.Repeat("<&", 100)} {
		raw, err := json.Marshal(text)
		if err != nil {
			t.Fatal(err)
		}
		if got := cacheJSONStringSize(text, int64(len(raw))); got != int64(len(raw)) {
			t.Fatalf("string size differs: estimate=%d actual=%d text=%x JSON=%q", got, len(raw), text, raw)
		}
	}
}

func TestCache_RequestHandlesOverrideEngineDefaults(t *testing.T) {
	defaults, scoped := &memoryCache{}, &memoryCache{}
	e := &Engine{release: Release{Rel: "20990101"}, options: Options{Documents: defaults, Sequences: defaults}}
	key := documentKey(e.release.Rel, "vn:1")
	defaults.Put(e.release.Rel, key, metadataJSON(DocumentEntry{Title: "default"}))
	scoped.Put(e.release.Rel, key, metadataJSON(DocumentEntry{Title: "scoped"}))
	ctx := WithCaches(t.Context(), scoped, scoped)
	docs, err := e.Documents(ctx, []string{"vn:1"})
	if err != nil || docs["vn:1"].Title != "scoped" {
		t.Fatal("document handles were not request scoped", err)
	}
	seq := readSequence{doc: "scoped", units: []Unit{{Kind: "provision", Text: "scoped"}}, complete: true}
	c := cursor{Item: ReadItem{Handle: "vn:1#p1"}}
	e.saveSequence(ctx, c, seq)()
	loaded, code, _, err := e.sequence(ctx, nil, c)
	if err != nil || code != "" || !reflect.DeepEqual(loaded.units, seq.units) || defaults.puts != 1 {
		t.Fatal("sequence handles were not request scoped", err)
	}
}
