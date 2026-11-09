package retrieval

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func shapeFixture(text string) (Arms, Recipe) {
	id := Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "clause"}
	article := Provision{ID: 1, NodeID: "a", Kind: "dieu", Label: "Điều 1", Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "article"}}
	clause := Provision{ID: 2, NodeID: "c", Kind: "khoan", Label: "Khoản 1", Identity: id, Text: text}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	c := Candidate{ChunkID: 1, GroupID: 2, ProvisionID: 2, Group: id, Identity: id, Provision: clause, Ancestors: []Provision{article, clause}, Descendants: []Provision{clause}, Document: Document{StatusClass: "in_force", IsCurrent: true, EffectiveFrom: &start}, Cosine: .5}
	a := rankFixture([]Candidate{c}, nil)
	a.Date = Date{2025, time.January, 1}
	recipe := Recipe1()
	threshold := .5
	recipe.Abstention.Threshold = &threshold
	return a, recipe
}

func TestAbstain_EqualityPasses(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Abstain {
		t.Fatalf("unexpected abstention: %+v", got)
	}
}
func TestAbstain_NoPrimaryHits(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	a.Primary = ArmGroup{}
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Abstain {
		t.Fatal("no primary hits must abstain")
	}
}
func TestEvidence_FullTextWithinCap(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Text != "Nội dung" || !got.Hits[0].Complete {
		t.Fatalf("hits: %+v", got.Hits)
	}
}
func TestEvidence_CutAtCapMarksIncompleteWithNextPart(t *testing.T) {
	a, r := shapeFixture(strings.Repeat("á", 14001))
	c := &a.Primary.Dense[0]
	c.Content = c.Provision.Text
	c.Spans = []byte(`[{"node_id":"c","start":0,"end":14001,"content_start":0,"content_end":14001,"role":"body"}]`)
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	h := got.Hits[0]
	if len([]rune(h.Text)) != 14000 || h.Complete || h.NextPart == nil || *h.NextPart != 1 {
		t.Fatalf("cut: length=%d complete=%t next=%v", len([]rune(h.Text)), h.Complete, h.NextPart)
	}
}
func TestCitation_FromAncestryLabels(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hits[0].Citation != "Điều 1, Khoản 1" {
		t.Fatal(got.Hits[0].Citation)
	}
}
func TestValidity_MissingStartGapAndIndeterminateCurrent(t *testing.T) {
	a, r := shapeFixture("Nội dung")
	c := &a.Primary.Dense[0]
	c.Document.EffectiveFrom = nil
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Hits[0].Stamp.Current || len(got.Hits[0].Gaps) == 0 {
		t.Fatalf("hit %+v", got.Hits[0])
	}
	a.AsOf = &Date{2021, time.January, 1}
	got, err = Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hits[0].Stamp.Current {
		t.Fatal("indeterminate historical current")
	}
}

func TestEvidence_AncestorLeadFirstAndCutLeadIncomplete(t *testing.T) {
	a, r := shapeFixture("")
	c := &a.Primary.Dense[0]
	c.Ancestors[0].Text = strings.Repeat("á", 14001)
	c.Content = c.Ancestors[0].Text
	c.Spans = []byte(`[{"node_id":"a","start":0,"end":14001,"content_start":0,"content_end":14001,"role":"lead"}]`)
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	h := got.Hits[0]
	if h.Complete || h.NextPart == nil || len(h.Fragments) != 1 || h.Fragments[0].Complete {
		t.Fatalf("lead incomplete: %+v", h)
	}
}

func TestEvidence_FragmentCompleteOnlyWhenAllIntervals(t *testing.T) {
	a, r := shapeFixture(strings.Repeat("a", 14001))
	c := &a.Primary.Dense[0]
	child := Provision{ID: 3, NodeID: "p", Kind: "diem", Text: strings.Repeat("a", 14001), Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "point"}}
	p := int64(2)
	child.ParentID = &p
	c.Descendants = append(c.Descendants, child)
	c.Content = c.Provision.Text
	c.Spans = []byte(`[{"node_id":"c","start":0,"end":14001,"content_start":0,"content_end":14001,"role":"body"},{"node_id":"p","start":0,"end":14001,"content_start":0,"content_end":14001,"role":"body"}]`)
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got.Hits[0].Fragments {
		if f.Identity.Anchor == "point" && f.Complete {
			t.Fatal("cut child credited complete")
		}
	}
}

func TestValidity_NonCurrentGroupUsesRemainingSlots(t *testing.T) {
	a, r := shapeFixture("one")
	b, _ := shapeFixture("two")
	c := b.Primary.Dense[0]
	c.ChunkID = 2
	c.GroupID = 3
	c.Group.Anchor = "other"
	c.Descendants[0].ID = 3
	c.Document.StatusClass = "expired"
	c.Document.IsCurrent = false
	b.Primary.Dense[0] = c
	b.Primary.Members[0] = c
	a.IncludeNonCurrent = true
	a.NonCurrent = b.Primary
	got, err := Shape(a, r, 1, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || len(got.NonCurrent) != 0 {
		t.Fatalf("unexpected slots: %+v", got)
	}
	got, err = Shape(a, r, 2, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NonCurrent) != 1 || got.NonCurrent[0].Stamp.Current {
		t.Fatalf("noncurrent: %+v", got.NonCurrent)
	}
}

func TestRelations_CappedSortedWithOmissionGap(t *testing.T) {
	a, r := shapeFixture("text")
	c := &a.Primary.Dense[0]
	for i := 20; i >= 0; i-- {
		c.Relations = append(c.Relations, SourceRelation{ArtifactSHA256: "hash", Ordinal: i, Value: Relation{Kind: "cites", SourceKey: "vn:1", Evidence: strings.Repeat("á", 1001)}})
	}
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	h := got.Hits[0]
	if len(h.Relations) != 20 || len([]rune(h.Relations[0].Evidence)) != 1000 || h.Relations[0].Complete || len(h.Relations[0].EvidenceSHA256) != 64 || len(h.Gaps) == 0 || h.Gaps[0].Kind != "relations_omitted" {
		t.Fatalf("relations: %+v", h)
	}
}

func TestEvidence_FragmentsSliceHitText(t *testing.T) {
	a, r := shapeFixture("Mở đầu\nđiểm con")
	c := &a.Primary.Dense[0]
	parent := int64(2)
	child := Provision{ID: 3, ParentID: &parent, NodeID: "p", Kind: "diem", Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "point"}, Text: "điểm con"}
	c.Descendants = append(c.Descendants, child)
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	h := got.Hits[0]
	seen := false
	for _, f := range h.Fragments {
		if f.Identity.Anchor == "point" {
			seen = true
			if string([]rune(h.Text)[f.TextStart:f.TextEnd]) != "điểm con" || !f.Complete {
				t.Fatalf("fragment %+v", f)
			}
		}
	}
	if !seen {
		t.Fatal("missing child fragment")
	}
}
func TestEvidence_AmbiguousParentNoChildCredit(t *testing.T) {
	a, r := shapeFixture("child child")
	c := &a.Primary.Dense[0]
	parent := int64(2)
	c.Descendants = append(c.Descendants, Provision{ID: 3, ParentID: &parent, NodeID: "p", Text: "child", Identity: Identity{Anchor: "point"}})
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	h := got.Hits[0]
	for _, f := range h.Fragments {
		if f.Identity.Anchor == "point" {
			t.Fatal("ambiguous child credited")
		}
	}
	if len(h.Gaps) == 0 || h.Gaps[0].Kind != "ambiguous_tree_text" {
		t.Fatal("missing ambiguity gap")
	}
}
func TestAbstain_FirstGroupBestCosineIncludingLexicalOnly(t *testing.T) {
	a, r := shapeFixture("text")
	c := a.Primary.Dense[0]
	c.Cosine = .9
	a.Primary.Dense = nil
	a.Primary.Lexical = []Candidate{c}
	a.Primary.Members = []Candidate{c}
	threshold := .8
	r.Abstention.Threshold = &threshold
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Abstain || got.Hits[0].BestCosine != .9 {
		t.Fatalf("result %+v", got)
	}
}
func TestValidity_DefaultCurrentAndExplicitAsOf(t *testing.T) {
	a, r := shapeFixture("text")
	c := &a.Primary.Dense[0]
	c.Document.StatusClass = "expired"
	c.Document.IsCurrent = false
	a.Primary.Members[0] = *c
	a.AsOf = &Date{2021, time.January, 1}
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Hits[0].Stamp.Current || got.Hits[0].Stamp.Basis != "document_dates" || len(got.Gaps) == 0 {
		t.Fatalf("result %+v", got)
	}
	a.AsOf = nil
	got, err = Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hits[0].Stamp.Current {
		t.Fatal("expired status current")
	}
}

func TestEvidence_MultipleLeadIntervalsAreNotIndividuallyComplete(t *testing.T) {
	a, r := shapeFixture("body")
	c := &a.Primary.Dense[0]
	c.Ancestors[0].Text = "first gap second"
	c.Content = "first second"
	c.Spans = []byte(`[{"node_id":"a","start":0,"end":5,"content_start":0,"content_end":5,"role":"lead"},{"node_id":"a","start":10,"end":16,"content_start":6,"content_end":12,"role":"lead"}]`)
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got.Hits[0].Fragments {
		if f.Role == "lead" && f.Complete {
			t.Fatalf("incomplete interval credited: %+v", f)
		}
	}
}

func TestEvidence_ContiguousLeadSpansPreserveSourceText(t *testing.T) {
	a, r := shapeFixture("body")
	c := &a.Primary.Dense[0]
	c.Ancestors[0].Text = "abc"
	c.Content = "abc\nbody"
	c.Spans = []byte(`[{"node_id":"a","start":0,"end":1,"content_start":0,"content_end":1,"role":"lead"},{"node_id":"a","start":1,"end":2,"content_start":1,"content_end":2,"role":"lead"},{"node_id":"a","start":2,"end":3,"content_start":2,"content_end":3,"role":"lead"}]`)
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hits[0].Text != "abc\nbody" {
		t.Fatalf("text %q", got.Hits[0].Text)
	}
}

func TestShape_CacheInsufficientFails(t *testing.T) {
	a, r := shapeFixture("text")
	a.Primary.Members[0].ChunkID = 2
	_, err := Shape(a, r, 8, "hybrid")
	if !errors.Is(err, ErrCacheInsufficient) {
		t.Fatalf("error %v", err)
	}
}
func TestShape_SingleArmRemovesOtherArmBeforeFusion(t *testing.T) {
	a, r := shapeFixture("text")
	c := a.Primary.Dense[0]
	c.ChunkID = 2
	c.GroupID = 3
	c.Group.Anchor = "other"
	c.Descendants[0].ID = 3
	a.Primary.Lexical = []Candidate{c}
	a.Primary.Members = append(a.Primary.Members, c)
	got, err := Shape(a, r, 8, "lexical")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || got.Hits[0].Identity.Anchor != "other" {
		t.Fatalf("hits %+v", got.Hits)
	}
}

func TestShape_MissingTreeFails(t *testing.T) {
	a, r := shapeFixture("text")
	a.Primary.Dense[0].Descendants = nil
	a.Primary.Members[0].Descendants = nil
	_, err := Shape(a, r, 8, "hybrid")
	if !errors.Is(err, ErrCacheInsufficient) {
		t.Fatalf("error %v", err)
	}
}

func TestSearch_ValidationRejectionRetainsRelease(t *testing.T) {
	r := Recipe1()
	threshold := .5
	r.Abstention.Threshold = &threshold
	e := Engine{release: Release{Rel: "release-1", RetrievalRecipe: r}}
	got, err := e.Search(context.Background(), Request{Query: "ngan hang"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Abstain || got.Release != "release-1" || got.Reason != "diacritics_required" {
		t.Fatalf("response %+v", got)
	}
}

func TestEvidence_ContainedAliasRetainsCompleteChildOnCut(t *testing.T) {
	a, r := shapeFixture("abcXXXX")
	r.EvidenceRuneCap = 3
	c := &a.Primary.Dense[0]
	c.Content = "abcXXXX"
	parent := int64(2)
	c.Descendants = append(c.Descendants, Provision{ID: 3, ParentID: &parent, NodeID: "p", Text: "abc", Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "point"}})
	c.Spans = []byte(`[{"node_id":"c","start":0,"end":1,"content_start":0,"content_end":1,"role":"body"},{"node_id":"p","start":0,"end":1,"content_start":0,"content_end":1,"role":"body"},{"node_id":"c","start":1,"end":2,"content_start":1,"content_end":2,"role":"body"},{"node_id":"p","start":1,"end":2,"content_start":1,"content_end":2,"role":"body"},{"node_id":"c","start":2,"end":3,"content_start":2,"content_end":3,"role":"body"},{"node_id":"p","start":2,"end":3,"content_start":2,"content_end":3,"role":"body"}]`)
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	h := got.Hits[0]
	if h.Text != "abc" {
		t.Fatalf("text %q", h.Text)
	}
	for _, f := range h.Fragments {
		if f.Identity.Anchor == "point" && f.Complete {
			return
		}
	}
	t.Fatal("complete child alias was lost")
}

func TestEvidence_RejectsSpanSourceMismatch(t *testing.T) {
	a, r := shapeFixture("body")
	c := &a.Primary.Dense[0]
	c.Ancestors[0].Text = "abc"
	c.Content = "xyz"
	c.Spans = []byte(`[{"node_id":"a","start":0,"end":3,"content_start":0,"content_end":3,"role":"lead"}]`)
	a.Primary.Members[0] = *c
	if _, err := Shape(a, r, 8, "hybrid"); err == nil {
		t.Fatal("mismatched source span accepted")
	}
}

func TestValidity_UnknownStatusNeverCurrent(t *testing.T) {
	a, r := shapeFixture("text")
	c := &a.Primary.Dense[0]
	c.Document.StatusClass = "unknown"
	c.Document.IsCurrent = false
	a.Primary.Members[0] = *c
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hits[0].Stamp.Current {
		t.Fatal("unknown status current in default mode")
	}
}

func TestValidity_UnknownStatusHistoricalDateIsCurrent(t *testing.T) {
	a, r := shapeFixture("text")
	c := &a.Primary.Dense[0]
	c.Document.StatusClass = "unknown"
	c.Document.IsCurrent = false
	a.Primary.Members[0] = *c
	a.AsOf = &Date{2021, time.January, 1}
	got, err := Shape(a, r, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || !got.Hits[0].Stamp.Current || got.Hits[0].Stamp.Basis != "document_dates" {
		t.Fatalf("historical primary hits: %+v", got.Hits)
	}
}
