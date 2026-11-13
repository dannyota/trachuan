package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func siblingFixture(t *testing.T) (Arms, Recipe, *SiblingMerge) {
	t.Helper()
	a, r := shapeFixture("first clause")
	article := a.Primary.Dense[0].Ancestors[0]
	article.Text = "lead"
	makeClause := func(id int64, anchor, text string, order int) Candidate {
		c := a.Primary.Dense[0]
		p := c.Provision
		p.ID, p.ParentID, p.Identity.Anchor, p.NodeID, p.Text, p.Ordinal, p.Preorder = id, &article.ID, anchor, anchor, text, order, order
		c.ChunkID, c.GroupID, c.ProvisionID, c.Identity, c.Group, c.Provision = id, id, id, p.Identity, p.Identity, p
		c.Ancestors, c.Descendants = []Provision{article, p}, []Provision{p}
		c.Content = article.Text + "\n" + text
		c.Spans, _ = json.Marshal([]sourceSpan{{NodeID: article.NodeID, End: 4, ContentEnd: 4, Role: "lead"}, {NodeID: p.NodeID, End: len([]rune(text)), ContentStart: 5, ContentEnd: 5 + len([]rune(text)), Role: "body"}})
		return c
	}
	// Rank clause 2 first, but render clause 1 first.
	c2 := makeClause(3, "clause-2", "second clause", 2)
	c1 := makeClause(2, "clause-1", "first clause", 1)
	a.Primary = ArmGroup{Dense: []Candidate{c2, c1}, Members: []Candidate{c2, c1}}
	ac := Candidate{GroupID: article.ID, Group: article.Identity, Identity: article.Identity, Document: c1.Document, Ancestors: []Provision{article}, Descendants: []Provision{article, c1.Provision, c2.Provision}}
	return a, r, &SiblingMerge{Articles: map[int64]Candidate{article.ID: ac}}
}

func TestSiblingMerge_RankingRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		count  int
		veto   bool
		limit  int
		want   int
		merged bool
	}{
		{"two", 2, false, 8, 1, true}, {"one", 1, false, 8, 1, false}, {"article veto", 2, true, 8, 3, false}, {"outside window", 2, false, 1, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, r, _ := siblingFixture(t)
			a.Primary.Dense = a.Primary.Dense[:tc.count]
			if tc.veto {
				c := a.Primary.Dense[0]
				c.ChunkID, c.GroupID = 9, 1
				c.Group = c.Ancestors[0].Identity
				c.Ancestors = c.Ancestors[:1]
				a.Primary.Dense = append(a.Primary.Dense, c)
				a.Primary.Members = append(a.Primary.Members, c)
			}
			if tc.limit == 1 {
				r.HydrateMultiplier = 1
			}
			got, err := RankWithSiblingMerge(a, r, tc.limit, "dense", nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Groups) != tc.want || (got.Groups[0].Identity.Anchor == "article") != tc.merged {
				t.Fatalf("groups: %+v", got.Groups)
			}
		})
	}
}

func TestSiblingMerge_ScoreCosineAndCap(t *testing.T) {
	a, r, _ := siblingFixture(t)
	extra := a.Primary.Members[1]
	extra.ChunkID = 99
	extra.Cosine = .95
	a.Primary.Members = append(a.Primary.Members, extra)
	r.DocumentCap = 1
	wantScore := r.RRF.DenseWeight / float64(r.RRF.K+1)
	got, err := RankWithSiblingMerge(a, r, 8, "dense", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Score != wantScore || got.Groups[0].Best.ChunkID != 3 || got.Groups[0].BestCosine != .95 || *got.FirstBestCosine != .95 || len(got.Groups[0].Members) != 3 {
		t.Fatalf("ranking %+v", got)
	}
}

func TestSiblingMerge_TextAndCoverage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cap      int
		extra    bool
		text     string
		complete bool
		best     string
		start    int
	}{
		{"whole article", 100, false, "lead\nfirst clause\nsecond clause", true, "clause-2", 18},
		{"unmatched clause", 100, true, "lead\nfirst clause\nsecond clause", false, "clause-2", 18},
		{"rank order at cap", 20, false, "lead\nsecond clause\nf", false, "clause-2", 5},
		{"cut lead", 2, false, "le", false, "article", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, r, m := siblingFixture(t)
			r.EvidenceRuneCap = tc.cap
			if tc.extra {
				ac := m.Articles[1]
				ac.Descendants = append(ac.Descendants, Provision{ID: 4, ParentID: &ac.GroupID, Kind: "khoan", Identity: Identity{Anchor: "other"}, Text: "other"})
				m.Articles[1] = ac
			}
			out, err := m.Shape(a, r, 8, "dense", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Hits) != 1 {
				t.Fatalf("hits %+v", out.Hits)
			}
			h := out.Hits[0]
			if h.Text != tc.text || h.Complete != tc.complete || (h.NextPart == nil) != tc.complete || h.BestMember.Anchor != tc.best || h.BestTextStart != tc.start {
				t.Fatalf("hit %+v", h)
			}
			for _, f := range h.Fragments {
				if f.Identity.Anchor == "clause-1" && tc.cap == 20 && f.Complete {
					t.Fatal("cut member complete")
				}
			}
		})
	}
}

func TestSiblingMerge_PointBestAndAbsentBody(t *testing.T) {
	a, r, m := siblingFixture(t)
	c := a.Primary.Dense[0]
	point := Provision{ID: 8, ParentID: &c.GroupID, Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "point"}, NodeID: "point", Kind: "diem", Text: "second clause"}
	c.Identity = point.Identity
	c.ProvisionID = 8
	c.Descendants = append(c.Descendants, point)
	c.Spans = []byte(`[{"node_id":"a","start":0,"end":4,"content_start":0,"content_end":4,"role":"lead"},{"node_id":"point","start":0,"end":13,"content_start":5,"content_end":18,"role":"body"}]`)
	a.Primary.Dense[0] = c
	a.Primary.Members[0] = c
	out, err := m.Shape(a, r, 8, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Hits[0].BestMember != point.Identity || out.Hits[0].BestTextStart != 18 {
		t.Fatalf("hit %+v", out.Hits[0])
	}
	c.Spans = nil
	a.Primary.Dense[0] = c
	a.Primary.Members[0] = c
	out, err = m.Shape(a, r, 8, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Hits[0].BestMember.Anchor != "article" || out.Hits[0].BestTextStart != 0 {
		t.Fatalf("fallback %+v", out.Hits[0])
	}
}

func TestSiblingMerge_PartitionsAndFrozenWindow(t *testing.T) {
	a, r, m := siblingFixture(t)
	a.NonCurrent = a.Primary
	r.HydrateMultiplier = 1
	// The full limit admits both clauses; reserving a tail must not shrink that window.
	out, err := m.Shape(a, r, 2, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Hits) != 1 || len(out.NonCurrent) != 1 || out.Hits[0].Identity.Anchor != "article" || out.NonCurrent[0].Identity.Anchor != "article" {
		t.Fatalf("response %+v", out)
	}
	// A later clause cached for the article must never join a one-chunk window.
	out, err = m.Shape(a, r, 1, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Hits) != 1 || out.Hits[0].Identity.Anchor != "clause-2" {
		t.Fatalf("response %+v", out)
	}
}

func TestSiblingMerge_ArticleAttachments(t *testing.T) {
	a, r, m := siblingFixture(t)
	ac := m.Articles[1]
	ac.Warnings = []Warning{{Kind: "repeals"}}
	ac.WarningsOmitted = 2
	ac.Relations = []SourceRelation{{Value: Relation{Kind: "repeals", Evidence: "article relation"}}}
	m.Articles[1] = ac
	out, err := m.Shape(a, r, 8, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := out.Hits[0]
	if h.Citation != "Điều 1" || len(h.Path) != 1 || !reflect.DeepEqual(h.Warnings, ac.Warnings) || h.WarningsOmitted != 2 || len(h.Relations) != 1 || h.Relations[0].Evidence != "article relation" {
		t.Fatalf("hit %+v", h)
	}
}

func TestSiblingMerge_DefaultParity(t *testing.T) {
	a, r, _ := siblingFixture(t)
	before, _ := json.Marshal(r)
	recipeHash := r.Hash()
	for _, mode := range []ArmMode{"hybrid", "dense", "lexical"} {
		for _, limit := range []int{1, 8, 20} {
			want, err := Shape(a, r, limit, mode)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ShapeWithCap(a, r, limit, mode, nil)
			if err != nil {
				t.Fatal(err)
			}
			w, _ := json.Marshal(want)
			g, _ := json.Marshal(got)
			if string(w) != string(g) {
				t.Fatal("default shape changed")
			}
			rank, err := Rank(a, r, limit, mode)
			if err != nil {
				t.Fatal(err)
			}
			if len(rank.Groups) != len(got.Hits) {
				t.Fatal("rank/shape parity")
			}
			for i, g := range rank.Groups {
				if g.Identity != got.Hits[i].Identity {
					t.Fatal("rank/shape identity")
				}
			}
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) || recipeHash != r.Hash() {
		t.Fatal("recipe changed")
	}
}

func TestSiblingMerge_FrozenMembershipAndBackfill(t *testing.T) {
	a, r, m := siblingFixture(t)
	first := a.Primary.Dense[0]
	third := first
	third.ChunkID, third.GroupID = 4, 4
	third.Group.Anchor = "clause-3"
	third.Ancestors = append([]Provision(nil), first.Ancestors...)
	third.Ancestors[1].ID = 4
	a.Primary.Dense = append(a.Primary.Dense, third)
	a.Primary.Members = append(a.Primary.Members, third)
	r.HydrateMultiplier = 1
	got, err := RankWithSiblingMerge(a, r, 2, "dense", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Members) != 2 {
		t.Fatalf("late clause joined %+v", got)
	}
	// A cached article-owned group outside the window must not veto the merge.
	article := m.Articles[1]
	article.ChunkID = 8
	article.Cosine = .5
	a.Primary.Dense[2] = article
	a.Primary.Members = append(a.Primary.Members, article)
	got, err = RankWithSiblingMerge(a, r, 2, "dense", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Identity.Anchor != "article" {
		t.Fatalf("outside veto %+v", got)
	}
	// Count the merge once, keep the next document slot, then backfill demotions.
	a, r, _ = siblingFixture(t)
	r.DocumentCap = 1
	r.NonCurrentTail = 0
	own := a.Primary.Dense[0]
	own.ChunkID, own.GroupID = 20, 20
	own.Group.Anchor = "another-article"
	own.Ancestors = nil
	other := own
	other.ChunkID, other.GroupID = 21, 21
	other.Group.DocumentKey = "other"
	a.Primary.Dense = append(a.Primary.Dense, own, other)
	a.Primary.Members = append(a.Primary.Members, own, other)
	got, err = RankWithSiblingMerge(a, r, 3, "dense", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 3 || got.Groups[0].Identity.Anchor != "article" || got.Groups[1].Identity.DocumentKey != "other" || got.Groups[2].Identity.Anchor != "another-article" || len(got.Groups[0].siblings) != 2 {
		t.Fatalf("cap/backfill %+v", got)
	}
}

func TestSiblingMerge_ConfidenceUsesFirstResultBeforeCap(t *testing.T) {
	a, r, m := siblingFixture(t)
	later := a.Primary.Dense[0]
	later.ChunkID, later.GroupID = 20, 20
	later.Group.Anchor = "later"
	later.Cosine = .99
	later.Ancestors = nil
	later.Descendants = append([]Provision(nil), later.Descendants...)
	later.Descendants[0].ID = 20
	a.Primary.Dense = append(a.Primary.Dense, later)
	a.Primary.Members = append(a.Primary.Members, later)
	threshold := .6
	r.Abstention.Threshold = &threshold
	got, err := RankWithSiblingMerge(a, r, 8, "dense", nil)
	if err != nil {
		t.Fatal(err)
	}
	if *got.FirstBestCosine != .5 {
		t.Fatalf("statistic %+v", got)
	}
	out, err := m.Shape(a, r, 8, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Abstain || out.Reason != "low_confidence" {
		t.Fatalf("confidence %+v", out)
	}
}

func TestSiblingMerge_ArticleSubstantiveCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, articleText string
		lead              bool
		complete          bool
	}{
		{"contained", "lead\nfirst clause\nsecond clause", true, true},
		{"suffix", "lead\nfirst clause\nsecond clause\nsuffix duty", true, false},
		{"label only", "Điều 1", false, true},
		{"missing lead", "substantive condition", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, r, m := siblingFixture(t)
			ac := m.Articles[1]
			ac.Descendants = append([]Provision(nil), ac.Descendants...)
			ac.Descendants[0].Text = tc.articleText
			m.Articles[1] = ac
			if !tc.lead {
				for i := range a.Primary.Dense {
					a.Primary.Dense[i].Spans = nil
				}
				a.Primary.Members = append([]Candidate(nil), a.Primary.Dense...)
			}
			out, err := m.Shape(a, r, 8, "dense", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if out.Hits[0].Complete != tc.complete {
				t.Fatalf("complete %+v", out.Hits[0])
			}
		})
	}
}

func TestSiblingMerge_UnselectedNestedClauseNeverEarnsCredit(t *testing.T) {
	a, r, m := siblingFixture(t)
	c := a.Primary.Dense[0]
	nested := Provision{ID: 9, ParentID: &c.GroupID, Identity: Identity{Country: "vn", DocumentKey: "vn:1", Anchor: "unselected"}, NodeID: "nested", Kind: "khoan", Text: "nested clause"}
	c.Descendants = append(append([]Provision(nil), c.Descendants...), nested)
	a.Primary.Dense[0] = c
	a.Primary.Members[0] = c
	out, err := m.Shape(a, r, 8, "dense", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range out.Hits[0].Fragments {
		if f.Identity == nested.Identity && f.Complete {
			t.Fatal("unselected clause received complete fragment")
		}
	}
}

func TestSiblingMerge_HydrationAndCacheGuards(t *testing.T) {
	if _, err := (&Engine{}).HydrateSiblingMerge(context.Background(), Arms{}); err == nil {
		t.Fatal("serving engine accepted override")
	}
	a, r, _ := siblingFixture(t)
	if _, err := (&SiblingMerge{}).Shape(a, r, 8, "dense", nil, nil); !errors.Is(err, ErrCacheInsufficient) {
		t.Fatalf("missing article: %v", err)
	}
	if _, ok := reflect.TypeOf(Request{}).FieldByName("MergeSiblings"); ok {
		t.Fatal("serving exposes override")
	}
	if _, ok := reflect.TypeOf(Recipe{}).FieldByName("MergeSiblings"); ok {
		t.Fatal("recipe exposes override")
	}
}
