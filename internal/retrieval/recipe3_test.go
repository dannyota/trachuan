package retrieval

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecipe3_DefaultRanking(t *testing.T) {
	a, r, _ := siblingFixture(t)
	raw, _ := r.JSON()
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["version"] = 3
	fields["sibling_merge"] = "article-clauses/1"
	raw, _ = json.Marshal(fields)
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	got, err := Rank(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Identity.Anchor != "article" {
		t.Fatalf("recipe 3 did not merge: %+v", got.Groups)
	}
}

func TestRecipe2_PinnedJSONAndHash(t *testing.T) {
	r := Recipe2()
	raw, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	const wantJSON = `{"version":2,"dense_depth":50,"lexical_depth":50,"ann":{"pool_min":400,"pool_multiplier":24,"ef_max":1000,"iterative_scan":"strict_order","plan_cache_mode":"force_custom_plan","max_scan_tuples":20000,"scan_mem_multiplier":1,"fallback":"exact"},"rrf":{"k":60,"dense_weight":1,"lexical_weight":0.5,"routed_lexical_weight":1},"ties":"score-id","routing":"vn-number/1","group":"nearest-khoan","reducer":"max","hydrate_multiplier":8,"document_cap":5,"non_current_tail":2,"backfill":"demoted-order","family_collapse":"off-unverified-alignment","context_scope":"number-routed/1","validity":"document-dates/3","default_in_force_only":true,"query_gate":"vn-marks/1","evidence":"source-spans/1","evidence_rune_cap":14000,"relation_cap":20,"relation_evidence_rune_cap":1000,"abstention":{"statistic":"first-group-best-member-cosine","status":"provisional-old-product"}}`
	if string(raw) != wantJSON || r.Hash() != "108211da043b1de9c3da72f0306114af730b7d610cec2f5701e62307343e2922" {
		t.Fatalf("hash %s JSON %s", r.Hash(), raw)
	}
}

func TestRecipe3_PagingListsEachConstituentOnce(t *testing.T) {
	a, r, _ := siblingFixture(t)
	a.Release = "20990101_1"
	r.HydrateMultiplier = 1
	r.NonCurrentTail = 0
	pages, err := PagedHits(a, r, Request{Arms: "dense"}, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || len(pages[0].Hits) != 1 || len(pages[1].Hits) != 1 {
		t.Fatalf("pages: %+v", pages)
	}
	if pages[0].Hits[0].Identity.Anchor != "clause-2" || pages[1].Hits[0].Identity.Anchor != "clause-1" {
		t.Fatalf("anchors: %s %s", pages[0].Hits[0].Identity.Anchor, pages[1].Hits[0].Identity.Anchor)
	}
}

func TestSupportedRecipe_ExactPairs(t *testing.T) {
	for _, tc := range []struct {
		version int
		merge   string
		want    bool
	}{
		{2, "", true}, {3, "article-clauses/1", true}, {3, "", false}, {2, "article-clauses/1", false}, {1, "", false}, {4, "article-clauses/1", false}, {3, "unknown", false},
	} {
		r := Recipe2()
		r.Version, r.SiblingMerge = tc.version, tc.merge
		if got := supportedRecipe(r); got != tc.want {
			t.Errorf("version %d merge %q accepted %t", tc.version, tc.merge, got)
		}
	}
	r := Recipe3()
	r.DocumentCap++
	if supportedRecipe(r) {
		t.Fatal("accepted changed recipe values")
	}
}

func TestRecipe3_FieldOrderAndValues(t *testing.T) {
	r := Recipe3()
	r.Version, r.SiblingMerge = 2, ""
	if r != Recipe2() {
		t.Fatal("recipe 3 changed other values")
	}
	raw, err := Recipe3().JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"family_collapse":"off-unverified-alignment","sibling_merge":"article-clauses/1","context_scope"`) {
		t.Fatalf("field order: %s", raw)
	}
}

func TestRecipe3_PresentationUsesWinningBodyAndArticleNext(t *testing.T) {
	a, r, articles := siblingFixture(t)
	c := a.Primary.Dense[1]
	body := strings.Repeat("earlier clause text ", 80)
	c.Content = "lead\n" + body
	c.Provision.Text = body
	c.Descendants = []Provision{c.Provision}
	c.Spans, _ = json.Marshal([]sourceSpan{{NodeID: "a", End: 4, ContentEnd: 4, Role: "lead"}, {NodeID: c.Provision.NodeID, End: len([]rune(body)), ContentStart: 5, ContentEnd: 5 + len([]rune(body)), Role: "body"}})
	a.Primary.Dense[1], a.Primary.Members[1] = c, c
	article := articles[1]
	article.Descendants = []Provision{article.Ancestors[0], c.Provision, a.Primary.Dense[0].Provision, {ID: 4, Kind: "khoan", Text: "unlisted"}}
	articles[1] = article
	response, err := Shape(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	h := response.Hits[0]
	if h.BestTextStart < SnippetCap {
		t.Fatal("fixture does not put best body beyond snippet cap")
	}
	compact := present(response, nil, DetailCompact).Hits[0]
	if !strings.Contains(compact.Snippet, "second clause") || strings.Contains(compact.Snippet, "earlier clause text") || compact.Complete {
		t.Fatalf("compact %+v", compact)
	}
	full := present(response, nil, DetailFull).Hits[0]
	if full.Complete || len(full.Fragments) < 3 || full.Next == "" || full.Handle != "vn:1#article" {
		t.Fatalf("full %+v", full)
	}
	next, err := decodeNext(full.Next)
	if err != nil || next.Item.Handle != full.Handle || next.Release != response.Release {
		t.Fatalf("article next %+v %v", next, err)
	}
}

func TestRecipe3_PagingKeepsUnlistedMembersInRankOrder(t *testing.T) {
	a, r, _ := siblingFixture(t)
	ranked, err := Rank(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	merged := ranked.Groups[0]
	first := merged.siblings[0]
	third := merged.siblings[1]
	third.Identity.Anchor = "clause-3"
	wider := merged
	wider.siblings = append(append([]Group(nil), merged.siblings...), third)
	got := prefixConstituents([]Group{first}, []Group{wider})
	if len(got) != 3 || got[0].Identity != first.Identity || got[1].Identity != merged.siblings[1].Identity || got[2].Identity != third.Identity {
		t.Fatal("partial merge lost member rank order")
	}
	if expanded := prefixConstituents([]Group{merged}, []Group{wider}); len(expanded) != 2 || expanded[1].Identity != third.Identity {
		t.Fatal("frozen membership hid a later constituent")
	}
	// A wider article-owned veto must not list an article already on page one.
	own := Group{Identity: merged.Identity}
	got = prefixConstituents([]Group{merged}, append([]Group{own}, merged.siblings...))
	if len(got) != 1 {
		t.Fatal("wider veto repeated listed article or clauses")
	}
}

func TestRecipe3_ArticleCapUsesReleaseGrouping(t *testing.T) {
	for _, merge := range []bool{false, true} {
		a, r, _ := siblingFixture(t)
		want := 1
		if !merge {
			r.Version, r.SiblingMerge = 2, ""
			want = 2
		}
		article := ArticleCapOverride(1)
		out, err := ShapeWithArticleCap(a, r, 8, "dense", nil, nil, &article)
		if err != nil || len(out.Hits) != want {
			t.Fatalf("merge %t hits %d error %v", merge, len(out.Hits), err)
		}
	}
}
