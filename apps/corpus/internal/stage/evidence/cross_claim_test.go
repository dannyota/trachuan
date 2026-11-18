package evidence

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/internal/retrieval"
)

const suspendSentence = "Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN ngày 30/12/2016."

var claimIssuers = map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"}

func suspendEnds() relationEnds {
	return relationEnds{TargetDocument: 2, TargetNode: "dieu-8/khoan-9", Kind: "suspends"}
}

func claimRelease(sourceText string) *fakeRelease {
	return &fakeRelease{
		documents: []releaseDocument{
			{ID: 1, Key: "vn:src", NumberNorm: retrieval.NormalizeNumber("10/2099/TT-NHNN")},
			{ID: 2, Key: "vn:target", NumberNorm: retrieval.NormalizeNumber("39/2016/TT-NHNN"), Issuer: claimIssuers["NHNN"], RawIssueDate: "30/12/2016"},
		},
		provisions: map[int64][]releaseProvision{
			1: {{ID: 10, NodeID: "a1", Kind: "dieu", Anchor: "dieu-1", Ordinal: 1, Text: sourceText}},
			2: {
				{ID: 20, NodeID: "t8", Kind: "dieu", Anchor: "dieu-8", Ordinal: 8, Text: "Điều 8"},
				{ID: 21, NodeID: "t8k9", Kind: "khoan", Anchor: "dieu-8/khoan-9", Ordinal: 9, Text: "9. text"},
			},
		},
		relations: map[relationEnds][]int64{},
	}
}

func detectClaim(t *testing.T, rel *fakeRelease) Observation {
	t.Helper()
	obs, err := detectClaims(context.Background(), rel, claimIssuers)
	if err != nil {
		t.Fatal(err)
	}
	return obs
}

func TestUnmatchedOperativeClaim_FiresWhenNoRelationRow(t *testing.T) {
	obs := detectClaim(t, claimRelease(suspendSentence))
	if len(obs.Findings) != 1 {
		t.Fatalf("findings %+v", obs.Findings)
	}
	f := obs.Findings[0]
	if f.Subject.Type != "claim" || f.Subject.Fields[0] != "vn:src" || f.Subject.Fields[1] != "dieu-1" || f.Subject.Fields[2] != "0" || f.Severity != "review" || f.Discriminator != "" {
		t.Fatalf("finding %+v", f)
	}
	if err := f.Subject.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUnmatchedOperativeClaim_RelationRowSilencesIt(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.relations[suspendEnds()] = []int64{10}
	if obs := detectClaim(t, rel); len(obs.Findings) != 0 {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_RowWithOtherKindOrTargetDoesNotSilence(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.relations[relationEnds{TargetDocument: 2, TargetNode: "dieu-8/khoan-9", Kind: "repeals"}] = []int64{10}
	rel.relations[relationEnds{TargetDocument: 2, TargetNode: "dieu-8/khoan-8", Kind: "suspends"}] = []int64{10}
	rel.relations[relationEnds{TargetDocument: 3, TargetNode: "dieu-8/khoan-9", Kind: "suspends"}] = []int64{10}
	if obs := detectClaim(t, rel); len(obs.Findings) != 1 {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_IncompleteSourceIsInfo(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.gaps = map[string]map[string]bool{"vn:src": {"ocr_flagged": true}}
	obs := detectClaim(t, rel)
	if len(obs.Findings) != 1 || obs.Findings[0].Severity != "info" {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_HeadingGapsStillReadAsReview(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.gaps = map[string]map[string]bool{"vn:src": {"tree_headings_partial": true}}
	obs := detectClaim(t, rel)
	if len(obs.Findings) != 1 || obs.Findings[0].Severity != "review" {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_UnresolvedTargetsAreNotClaims(t *testing.T) {
	cases := map[string]func(*fakeRelease){
		"target document absent": func(r *fakeRelease) { r.documents[1].NumberNorm = "other" },
		"ambiguous number": func(r *fakeRelease) {
			r.documents = append(r.documents, releaseDocument{ID: 3, Key: "vn:dup", NumberNorm: retrieval.NormalizeNumber("39/2016/TT-NHNN")})
		},
		"article missing": func(r *fakeRelease) { r.provisions[2] = nil },
		"sibling variant": func(r *fakeRelease) {
			r.provisions[2] = append(r.provisions[2], releaseProvision{ID: 22, NodeID: "t8b", Kind: "dieu", Anchor: "dieu-8~2", Ordinal: 8})
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			rel := claimRelease(suspendSentence)
			mutate(rel)
			if obs := detectClaim(t, rel); len(obs.Findings) != 0 {
				t.Fatalf("findings %+v", obs.Findings)
			}
		})
	}
}

func TestUnmatchedOperativeClaim_QuotedAndPlainTextAreIgnored(t *testing.T) {
	for _, text := range []string{
		"Thông tư này có hiệu lực từ ngày 01/01/2099.",
		"Sửa đổi quy định như sau: \"Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN ngày 30/12/2016.\"",
	} {
		if obs := detectClaim(t, claimRelease(text)); len(obs.Findings) != 0 {
			t.Errorf("%q: findings %+v", text, obs.Findings)
		}
	}
}

func TestUnmatchedOperativeClaim_OneFindingPerSentenceWithOrdinal(t *testing.T) {
	text := suspendSentence + " Tạm ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN."
	rel := claimRelease(text)
	obs := detectClaim(t, rel)
	if len(obs.Findings) != 2 || obs.Findings[0].Subject.Fields[2] != "0" || obs.Findings[1].Subject.Fields[2] != "1" {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_ChildNodesReadThroughParents(t *testing.T) {
	rel := claimRelease("")
	parent := int64(10)
	rel.provisions[1] = []releaseProvision{
		{ID: 10, NodeID: "a1", Kind: "dieu", Anchor: "dieu-1", Ordinal: 1, Text: "1. " + suspendSentence},
		{ID: 11, ParentID: &parent, NodeID: "a1k1", Kind: "khoan", Anchor: "dieu-1/khoan-1", Ordinal: 1, Text: suspendSentence},
	}
	if obs := detectClaim(t, rel); len(obs.Findings) != 1 || obs.Findings[0].Subject.Fields[1] != "dieu-1" {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_ObservedCountsAndCoverage(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.documents[1].Baseline = true
	obs := detectClaim(t, rel)
	if obs.Observed != 1 || obs.NotObservable != 1 {
		t.Fatalf("%+v", obs)
	}
	if !obs.ObservedCompletely(Subject{Type: "claim", Fields: []string{"vn:src", "dieu-1", "5"}}) || obs.ObservedCompletely(Subject{Type: "claim", Fields: []string{"vn:target", "dieu-8", "0"}}) {
		t.Fatal("claim coverage is wrong")
	}
}

func TestUnmatchedOperativeClaim_ReadsAnchorsOncePerTargetDocument(t *testing.T) {
	rel := claimRelease(suspendSentence + " Tạm ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN.")
	detectClaim(t, rel)
	if rel.queries != 1 {
		t.Fatalf("anchor queries %d", rel.queries)
	}
}

func TestUnmatchedOperativeClaim_ErrorsInsteadOfPartialResults(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.anchorsErr = context.DeadlineExceeded
	if _, err := detectClaims(context.Background(), rel, claimIssuers); err == nil {
		t.Fatal("no error")
	}
	if _, err := detectClaims(context.Background(), claimRelease(suspendSentence), nil); err == nil {
		t.Fatal("no error without the issuer table")
	}
}

func TestUnmatchedOperativeClaim_IdentityConflictIsNotAClaim(t *testing.T) {
	for name, mutate := range map[string]func(*releaseDocument){
		"other issuer":     func(d *releaseDocument) { d.Issuer = "BỘ TÀI CHÍNH" },
		"other issue date": func(d *releaseDocument) { d.RawIssueDate = "31/12/2016" },
		"no issue date":    func(d *releaseDocument) { d.RawIssueDate = "" },
	} {
		t.Run(name, func(t *testing.T) {
			rel := claimRelease(suspendSentence)
			mutate(&rel.documents[1])
			if obs := detectClaim(t, rel); len(obs.Findings) != 0 {
				t.Fatalf("findings %+v", obs.Findings)
			}
		})
	}
}

func TestUnmatchedOperativeClaim_ClassChangeKeepsKeyAndChangesFingerprint(t *testing.T) {
	read := detectClaim(t, claimRelease(suspendSentence)).Findings[0]
	rel := claimRelease(suspendSentence)
	rel.gaps = map[string]map[string]bool{"vn:src": {"ocr_flagged": true}}
	incomplete := detectClaim(t, rel).Findings[0]
	keyA, _ := findingKey(detector.UnmatchedOperativeClaim, read)
	keyB, _ := findingKey(detector.UnmatchedOperativeClaim, incomplete)
	fpA, _ := hashValue(read.FingerprintInputs)
	fpB, _ := hashValue(incomplete.FingerprintInputs)
	if keyA != keyB || fpA == fpB {
		t.Fatalf("key equal %v fingerprint equal %v", keyA == keyB, fpA == fpB)
	}
}

func TestUnmatchedOperativeClaim_UnorderedArticleIsNotObserved(t *testing.T) {
	const lead = "Điều 1. Sửa đổi, bổ sung một số điều của Thông tư 19/2018/TT-NHNN như sau:"
	const item = "1. Bãi bỏ khoản 2 Điều 5."
	rel := claimRelease("")
	parent := int64(10)
	rel.provisions[1] = []releaseProvision{
		{ID: 10, NodeID: "a1", Kind: "dieu", Anchor: "dieu-1", Ordinal: 1, Text: lead + "\n" + item},
		{ID: 11, ParentID: &parent, NodeID: "c1", Kind: "khoan", Anchor: "dieu-1/khoan-1", Ordinal: 1, Text: item},
		{ID: 12, ParentID: &parent, NodeID: "c2", Kind: "khoan", Anchor: "dieu-1/khoan-2", Ordinal: 2, Text: "2. Bãi bỏ Điều 6."},
		{ID: 13, NodeID: "a2", Kind: "dieu", Anchor: "dieu-2", Ordinal: 2, Text: "Điều 2."},
	}
	obs := detectClaim(t, rel)
	if obs.ObservedCompletely(Subject{Type: "claim", Fields: []string{"vn:src", "dieu-1", "0"}}) {
		t.Fatal("subject in an unread article counts as observed")
	}
	if !obs.ObservedCompletely(Subject{Type: "claim", Fields: []string{"vn:src", "dieu-2", "0"}}) {
		t.Fatal("subject in a read article is not observed")
	}
	if obs.Observed != 1 || obs.NotObservable != 1 {
		t.Fatalf("observed %d not observable %d", obs.Observed, obs.NotObservable)
	}
}

func TestUnmatchedOperativeClaim_EmptyDiscriminatorDoesNotCollide(t *testing.T) {
	text := suspendSentence + " Tạm ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN."
	obs := detectClaim(t, claimRelease(text))
	r := run(detector.UnmatchedOperativeClaim, 1, always)
	r.observation = obs
	r.spec.Severity = detector.Review
	if file := mustCompute(t, computeInput{Runs: []detectorRun{r}}); len(file.Findings) != 2 || file.Findings[0].Discriminator != "" {
		t.Fatalf("findings %+v", file.Findings)
	}
}

func TestUnmatchedOperativeClaim_MissingClauseResolvesToDeepestNode(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.provisions[2] = rel.provisions[2][:1]
	if obs := detectClaim(t, rel); len(obs.Findings) != 1 {
		t.Fatalf("findings %+v", obs.Findings)
	}
	rel.relations[suspendEnds()] = []int64{10}
	if obs := detectClaim(t, rel); len(obs.Findings) != 0 {
		t.Fatalf("row with the requested anchor did not close the claim: %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_RowOnSiblingClauseDoesNotSilence(t *testing.T) {
	rel := claimRelease(suspendSentence)
	rel.provisions[2] = rel.provisions[2][:1]
	rel.relations[relationEnds{TargetDocument: 2, TargetNode: "dieu-8/khoan-8", Kind: "suspends"}] = []int64{10}
	if obs := detectClaim(t, rel); len(obs.Findings) != 1 {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_EffectFromInsideTheArticleClosesIt(t *testing.T) {
	rel := claimRelease("")
	parent := int64(10)
	rel.provisions[1] = []releaseProvision{
		{ID: 10, NodeID: "a1", Kind: "dieu", Anchor: "dieu-1", Ordinal: 1, Text: "1. " + suspendSentence},
		{ID: 11, ParentID: &parent, NodeID: "a1k1", Kind: "khoan", Anchor: "dieu-1/khoan-1", Ordinal: 1, Text: suspendSentence},
	}
	rel.relations[suspendEnds()] = []int64{11}
	if obs := detectClaim(t, rel); len(obs.Findings) != 0 {
		t.Fatalf("clause-level row did not close the claim: %+v", obs.Findings)
	}
	other := claimRelease(suspendSentence)
	other.provisions[1] = append(other.provisions[1], releaseProvision{ID: 12, NodeID: "a2", Kind: "dieu", Anchor: "dieu-2", Ordinal: 2, Text: "Điều 2"})
	other.relations[suspendEnds()] = []int64{12}
	if obs := detectClaim(t, other); len(obs.Findings) != 1 {
		t.Fatalf("row from another article closed the claim: %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_OrdinalIsSentenceOrdinal(t *testing.T) {
	text := "Thông tư này có hiệu lực. " + suspendSentence
	obs := detectClaim(t, claimRelease(text))
	if len(obs.Findings) != 1 || obs.Findings[0].Subject.Fields[2] != "1" {
		t.Fatalf("findings %+v", obs.Findings)
	}
}

func TestUnmatchedOperativeClaim_SourcesRelateNeverReadAreInfo(t *testing.T) {
	for name, mutate := range map[string]func(*releaseDocument){
		"chinhphu":     func(d *releaseDocument) { d.Source = "chinhphu" },
		"consolidated": func(d *releaseDocument) { d.Consolidated = true },
	} {
		t.Run(name, func(t *testing.T) {
			rel := claimRelease(suspendSentence)
			mutate(&rel.documents[0])
			obs := detectClaim(t, rel)
			if len(obs.Findings) != 1 || obs.Findings[0].Severity != "info" {
				t.Fatalf("findings %+v", obs.Findings)
			}
		})
	}
}

func claimFile(t *testing.T, rel string, previous *File, obs Observation) File {
	t.Helper()
	r := run(detector.UnmatchedOperativeClaim, 1, obs.ObservedCompletely, obs.Findings...)
	r.spec.Severity = detector.Review
	return mustCompute(t, computeInput{Rel: rel, Runs: []detectorRun{r}, Previous: previous})
}

func TestUnmatchedOperativeClaim_LostInputCarriesThePreviousFinding(t *testing.T) {
	first := claimFile(t, "20990101_1", nil, detectClaim(t, claimRelease(suspendSentence)))
	if len(first.Findings) != 1 {
		t.Fatalf("first run %+v", first.Findings)
	}
	const lead = "Điều 1. Sửa đổi, bổ sung một số điều của Thông tư 19/2018/TT-NHNN như sau:"
	cases := map[string]func(*fakeRelease){
		"target document gone": func(r *fakeRelease) { r.documents = r.documents[:1] },
		"number ambiguous": func(r *fakeRelease) {
			r.documents = append(r.documents, releaseDocument{ID: 3, Key: "vn:dup", NumberNorm: r.documents[1].NumberNorm})
		},
		"anchors missing":   func(r *fakeRelease) { r.provisions[2] = nil },
		"identity conflict": func(r *fakeRelease) { r.documents[1].RawIssueDate = "31/12/2016" },
		"source needs ocr": func(r *fakeRelease) {
			r.gaps = map[string]map[string]bool{"vn:src": {"needs_ocr": true}}
			r.provisions[1] = nil
		},
		"source flagged text": func(r *fakeRelease) {
			r.gaps = map[string]map[string]bool{"vn:src": {"ocr_rejected": true}}
			r.provisions[1][0].Text = "Điều 1."
		},
		"article emptied under a heading gap": func(r *fakeRelease) {
			r.gaps = map[string]map[string]bool{"vn:src": {"tree_headings_partial": true}}
			r.provisions[1][0].Text = ""
		},
		"article unordered": func(r *fakeRelease) {
			p := int64(10)
			r.provisions[1] = []releaseProvision{
				{ID: 10, NodeID: "a1", Kind: "dieu", Anchor: "dieu-1", Ordinal: 1, Text: lead + "\n1. Bãi bỏ khoản 2 Điều 5."},
				{ID: 11, ParentID: &p, NodeID: "c1", Kind: "khoan", Anchor: "dieu-1/khoan-1", Ordinal: 1, Text: "1. Bãi bỏ khoản 2 Điều 5."},
				{ID: 12, ParentID: &p, NodeID: "c2", Kind: "khoan", Anchor: "dieu-1/khoan-2", Ordinal: 2, Text: "2. Bãi bỏ Điều 6."},
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			rel := claimRelease(suspendSentence)
			mutate(rel)
			second := claimFile(t, "20990102_1", &first, detectClaim(t, rel))
			if len(second.Gone) != 0 || len(second.Carried) != 1 {
				t.Fatalf("gone %v carried %v findings %+v", second.Gone, second.Carried, second.Findings)
			}
		})
	}
}

func TestUnmatchedOperativeClaim_ClosedClaimIsGone(t *testing.T) {
	first := claimFile(t, "20990101_1", nil, detectClaim(t, claimRelease(suspendSentence)))
	rel := claimRelease(suspendSentence)
	rel.relations[suspendEnds()] = []int64{10}
	second := claimFile(t, "20990102_1", &first, detectClaim(t, rel))
	if len(second.Gone) != 1 || len(second.Carried) != 0 {
		t.Fatalf("gone %v carried %v", second.Gone, second.Carried)
	}
}
