package knowledge

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

type fakeDB struct {
	documents  map[string]Document
	baseline   []string
	provisions map[int64][]Provision
	texts      map[int64]string
	rows       []RelationRow
}

func (f *fakeDB) Documents(_ context.Context, keys []string) ([]Document, error) {
	var out []Document
	for _, key := range keys {
		if d, ok := f.documents[key]; ok {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeDB) BaselineServed(_ context.Context, keys []string) ([]string, error) {
	var out []string
	for _, key := range f.baseline {
		if slices.Contains(keys, key) {
			out = append(out, key)
		}
	}
	return out, nil
}

func (f *fakeDB) RelationRows(_ context.Context, ids []int64) ([]RelationRow, error) {
	var out []RelationRow
	for _, row := range f.rows {
		if slices.Contains(ids, row.SourceDocumentID) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeDB) Provisions(_ context.Context, id int64) ([]Provision, error) {
	return f.provisions[id], nil
}

func (f *fakeDB) ProvisionText(_ context.Context, id int64) (string, error) { return f.texts[id], nil }

const (
	targetKey = "vn:vbpl:1"
	actingKey = "vn:vbpl:2"
	quote     = "Bãi bỏ Điều 5 của văn bản đích."
)

func id64(v int64) *int64 { return &v }

func sp(v string) *string { return &v }

func corpus() *fakeDB {
	return &fakeDB{
		documents: map[string]Document{
			targetKey: {ID: 1, Key: targetKey, Number: "1/2099/ND-CP", NumberNorm: "1/2099/nd-cp", TextSource: "source_tree", TextComplete: true},
			actingKey: {ID: 2, Key: actingKey, Number: "2/2099/ND-CP", NumberNorm: "2/2099/nd-cp", TextSource: "pdf_text", TextComplete: true},
		},
		provisions: map[int64][]Provision{
			1: {{ID: 10, NodeID: "t-chuong", Anchor: "chuong-1", Heading: "Chương một"}, {ID: 11, ParentID: id64(10), NodeID: "t-dieu", Anchor: "dieu-5", Heading: "Điều năm"}},
			2: {{ID: 20, NodeID: "a-dieu", Anchor: "dieu-30", Heading: "Điều ba mươi"}, {ID: 21, ParentID: id64(20), NodeID: "a-khoan", Anchor: "dieu-30/khoan-2"}},
		},
		texts: map[int64]string{10: "", 11: "Văn bản đích.", 20: "Toàn văn điều.", 21: "Khoản 2. " + quote},
	}
}

func effectClaim(t *testing.T, db *fakeDB) Claim {
	t.Helper()
	ctx := context.Background()
	acting, err := Depends(ctx, db, actingKey, "dieu-30/khoan-2")
	if err != nil {
		t.Fatal(err)
	}
	target, err := Depends(ctx, db, targetKey, "dieu-5")
	if err != nil {
		t.Fatal(err)
	}
	return Claim{ID: "effect-mau", Kind: KindEffect, Position: 4, ActingKey: actingKey, ActingAnchor: "dieu-30/khoan-2", TargetKey: targetKey, TargetAnchor: "dieu-5", RelationKind: "repeals", Quote: quote, Contract: ContractVersion, ActingDepends: acting, TargetDepends: target, TargetNumberNorm: target.NumberNorm}
}

func relationClaim(t *testing.T, db *fakeDB) Claim {
	t.Helper()
	acting, err := Depends(context.Background(), db, actingKey, "dieu-30/khoan-2")
	if err != nil {
		t.Fatal(err)
	}
	return Claim{ID: "relation-mau", Kind: KindRelation, Position: 5, ActingKey: actingKey, ActingAnchor: "dieu-30/khoan-2", TargetKey: targetKey, RelationKind: "replaces", Quote: quote, Contract: ContractVersion, ActingDepends: acting, TargetNumberNorm: "1/2099/nd-cp"}
}

func TestDepends_ComputesFromStoredColumns(t *testing.T) {
	got, err := Depends(context.Background(), corpus(), targetKey, "dieu-5")
	if err != nil {
		t.Fatal(err)
	}
	if got.NumberNorm != "1/2099/nd-cp" || got.TextSource != "source_tree" || got.TextSHA256 != Hash("Văn bản đích.") || got.HeadingSHA256 != Hash("Điều năm") {
		t.Fatalf("depends = %+v", got)
	}
	if !slices.Equal(got.Ancestors, []Ancestor{{"chuong-1", Hash("Chương một")}}) {
		t.Fatalf("ancestors = %+v", got.Ancestors)
	}
	if Hash("") != "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("empty hash = %s", Hash(""))
	}
}

func TestDepends_YAMLParsesBack(t *testing.T) {
	db := corpus()
	want := effectClaim(t, db).ActingDepends
	data, err := want.YAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "text_sha256: "+want.TextSHA256+"\n") || !strings.Contains(string(data), "number_norm: 2/2099/nd-cp\n") {
		t.Fatalf("block:\n%s", data)
	}
	indented := "\n      " + strings.ReplaceAll(strings.TrimSpace(string(data)), "\n", "\n      ") + "\n"
	entry := strings.Replace(validEffect("effect-mau"), "    acting:"+validProvision("      "), "    acting:"+indented, 1)
	file, err := Parse("vn", []byte(entry))
	if err != nil {
		t.Fatalf("%v\n%s", err, entry)
	}
	got := file.Claims[0].ActingDepends
	if got.TextSHA256 != want.TextSHA256 || got.NumberNorm != want.NumberNorm || !slices.Equal(got.Ancestors, want.Ancestors) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestDepends_Refuses(t *testing.T) {
	for name, tc := range map[string]struct {
		edit        func(*fakeDB)
		key, anchor string
		want        string
	}{
		"absent document":  {nil, "vn:vbpl:9", "dieu-1", ReasonDocumentAbsent},
		"absent anchor":    {nil, targetKey, "dieu-9", ReasonAnchorAbsent},
		"suffixed anchor":  {nil, targetKey, "dieu-5~2", "~k"},
		"ambiguous":        {func(db *fakeDB) { db.provisions[1] = append(db.provisions[1], Provision{ID: 12, Anchor: "dieu-5~2"}) }, targetKey, "dieu-5", ReasonAmbiguous},
		"ambiguous parent": {func(db *fakeDB) { db.provisions[2] = append(db.provisions[2], Provision{ID: 22, Anchor: "dieu-30~2"}) }, actingKey, "dieu-30/khoan-2", ReasonAmbiguous},
	} {
		t.Run(name, func(t *testing.T) {
			db := corpus()
			if tc.edit != nil {
				tc.edit(db)
			}
			if _, err := Depends(context.Background(), db, tc.key, tc.anchor); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDepends_PrintsForBaselineAndIncompleteDocuments(t *testing.T) {
	db := corpus()
	db.baseline = []string{targetKey}
	d := db.documents[targetKey]
	d.TextComplete = false
	db.documents[targetKey] = d
	got, err := Depends(context.Background(), db, targetKey, "dieu-5")
	if err != nil || got.TextSHA256 != Hash("Văn bản đích.") {
		t.Fatalf("depends = %+v, %v", got, err)
	}
}

type claimCase struct {
	name     string
	relation bool
	edit     func(*fakeDB, *Claim)
	outcome  string
	reason   string
	problem  string
}

func TestCheck_CaseTable(t *testing.T) {
	matching := func(kind, method string) func(*fakeDB, *Claim) {
		return func(db *fakeDB, _ *Claim) {
			db.rows = append(db.rows, RelationRow{Kind: kind, Method: method, SourceDocumentID: 2, TargetDocumentID: 1, SourceProvisionID: id64(21), TargetProvisionID: id64(11), TargetNodeID: sp("dieu-5")})
		}
	}
	documentRow := func(kind string) func(*fakeDB, *Claim) {
		return func(db *fakeDB, _ *Claim) {
			db.rows = append(db.rows, RelationRow{Kind: kind, Method: "diagram", SourceDocumentID: 2, TargetDocumentID: 1})
		}
	}
	doc := func(key string, edit func(*Document)) func(*fakeDB, *Claim) {
		return func(db *fakeDB, _ *Claim) {
			d := db.documents[key]
			edit(&d)
			db.documents[key] = d
		}
	}
	for _, tc := range []claimCase{
		{name: "effect applied", outcome: OutcomeApplied},
		{name: "relation applied", relation: true, outcome: OutcomeApplied},
		{name: "acting absent", edit: func(db *fakeDB, _ *Claim) { delete(db.documents, actingKey) }, outcome: OutcomeStale, reason: "document_absent"},
		{name: "target absent", edit: func(db *fakeDB, _ *Claim) { delete(db.documents, targetKey) }, outcome: OutcomeStale, reason: "document_absent"},
		{name: "relation target absent", relation: true, edit: func(db *fakeDB, _ *Claim) { delete(db.documents, targetKey) }, outcome: OutcomeStale, reason: "document_absent"},
		{name: "acting baseline", edit: func(db *fakeDB, _ *Claim) { db.baseline = []string{actingKey} }, outcome: OutcomeStale, reason: "baseline"},
		{name: "relation target baseline", relation: true, edit: func(db *fakeDB, _ *Claim) { db.baseline = []string{targetKey} }, outcome: OutcomeStale, reason: "baseline"},
		{name: "absent target beats acting baseline", edit: func(db *fakeDB, _ *Claim) { db.baseline = []string{actingKey}; delete(db.documents, targetKey) }, outcome: OutcomeStale, reason: "document_absent"},
		{name: "target incomplete", edit: doc(targetKey, func(d *Document) { d.TextComplete = false }), outcome: OutcomeStale, reason: "incomplete"},
		{name: "relation target incomplete is not named", relation: true, edit: doc(targetKey, func(d *Document) { d.TextComplete = false }), outcome: OutcomeApplied},
		{name: "relation acting incomplete", relation: true, edit: doc(actingKey, func(d *Document) { d.TextComplete = false }), outcome: OutcomeStale, reason: "incomplete"},
		{name: "anchor absent", edit: func(_ *fakeDB, c *Claim) { c.TargetAnchor = "dieu-9" }, outcome: OutcomeStale, reason: "anchor_absent"},
		{name: "sibling makes ambiguous", edit: func(db *fakeDB, _ *Claim) {
			db.provisions[1] = append(db.provisions[1], Provision{ID: 12, Anchor: "dieu-5~2"})
		}, outcome: OutcomeStale, reason: "ambiguous"},
		{name: "ancestor sibling makes ambiguous", edit: func(db *fakeDB, _ *Claim) {
			db.provisions[1] = append(db.provisions[1], Provision{ID: 12, Anchor: "chuong-1~2"})
		}, outcome: OutcomeStale, reason: "ambiguous"},
		{name: "path component sibling makes ambiguous", edit: func(db *fakeDB, _ *Claim) {
			db.provisions[2] = append(db.provisions[2], Provision{ID: 22, Anchor: "dieu-30~3"})
		}, outcome: OutcomeStale, reason: "ambiguous"},
		{name: "suffixed ancestor makes ambiguous", edit: func(db *fakeDB, _ *Claim) { db.provisions[1][0].Anchor = "chuong-1~2" }, outcome: OutcomeStale, reason: "ambiguous"},
		{name: "relation target sibling is not named", relation: true, edit: func(db *fakeDB, _ *Claim) {
			db.provisions[1] = append(db.provisions[1], Provision{ID: 12, Anchor: "dieu-5~2"})
		}, outcome: OutcomeApplied},
		{name: "contract", edit: func(_ *fakeDB, c *Claim) { c.Contract = 2 }, outcome: OutcomeStale, reason: "contract"},
		{name: "contract beats number", edit: func(_ *fakeDB, c *Claim) { c.Contract = 2; c.ActingDepends.NumberNorm = "x" }, outcome: OutcomeStale, reason: "contract"},
		{name: "acting number", edit: func(_ *fakeDB, c *Claim) { c.ActingDepends.NumberNorm = "x" }, outcome: OutcomeStale, reason: "number_norm"},
		{name: "target number", edit: func(_ *fakeDB, c *Claim) { c.TargetDepends.NumberNorm = "x"; c.TargetNumberNorm = "x" }, outcome: OutcomeStale, reason: "number_norm"},
		{name: "relation target number", relation: true, edit: func(_ *fakeDB, c *Claim) { c.TargetNumberNorm = "x" }, outcome: OutcomeStale, reason: "number_norm"},
		{name: "number beats text", edit: func(_ *fakeDB, c *Claim) { c.TargetNumberNorm = "x"; c.ActingDepends.TextSHA256 = testHash("f") }, outcome: OutcomeStale, reason: "number_norm"},
		{name: "text source", edit: func(_ *fakeDB, c *Claim) { c.TargetDepends.TextSource = "ocr" }, outcome: OutcomeStale, reason: "text_source"},
		{name: "text", edit: func(_ *fakeDB, c *Claim) { c.TargetDepends.TextSHA256 = testHash("f") }, outcome: OutcomeStale, reason: "text"},
		{name: "heading", edit: func(_ *fakeDB, c *Claim) { c.ActingDepends.HeadingSHA256 = testHash("f") }, outcome: OutcomeStale, reason: "heading"},
		{name: "ancestors", edit: func(_ *fakeDB, c *Claim) { c.TargetDepends.Ancestors = nil }, outcome: OutcomeStale, reason: "ancestors"},
		{name: "ancestors order", edit: func(_ *fakeDB, c *Claim) {
			c.ActingDepends.Ancestors = []Ancestor{{"dieu-31", Hash("Điều ba mươi")}}
		}, outcome: OutcomeStale, reason: "ancestors"},
		{name: "quote absent", edit: func(_ *fakeDB, c *Claim) { c.Quote = "không có trong điều này" }, problem: "quote"},
		{name: "quote needs exact white space", edit: func(_ *fakeDB, c *Claim) { c.Quote = strings.Replace(quote, " ", "  ", 1) }, problem: "quote"},
		{name: "stale wins over bad quote", edit: func(_ *fakeDB, c *Claim) { c.Quote = "không có"; c.Contract = 2 }, outcome: OutcomeStale, reason: "contract"},
		{name: "effect redundant", edit: matching("repeals", "text_rule/1"), outcome: OutcomeRedundant},
		{name: "reviewed row is not matching", edit: matching("repeals", MethodReviewed), outcome: OutcomeApplied},
		{name: "reviewed other kind does not contradict", edit: matching("suspends", MethodReviewed), outcome: OutcomeApplied},
		{name: "effect other kind", edit: matching("suspends", "text_rule/1"), problem: "suspends"},
		{name: "same kind wins over other kind", edit: func(db *fakeDB, c *Claim) { matching("suspends", "x")(db, c); matching("repeals", "x")(db, c) }, outcome: OutcomeRedundant},
		{name: "row on another target document", edit: func(db *fakeDB, _ *Claim) {
			db.rows = append(db.rows, RelationRow{Kind: "suspends", Method: "x", SourceDocumentID: 2, TargetDocumentID: 3, SourceProvisionID: id64(21), TargetNodeID: sp("dieu-5")})
		}, outcome: OutcomeApplied},
		{name: "row from another provision", edit: func(db *fakeDB, _ *Claim) {
			db.rows = append(db.rows, RelationRow{Kind: "suspends", Method: "x", SourceDocumentID: 2, TargetDocumentID: 1, SourceProvisionID: id64(20), TargetNodeID: sp("dieu-5")})
		}, outcome: OutcomeApplied},
		{name: "relation redundant", relation: true, edit: documentRow("replaces"), outcome: OutcomeRedundant},
		{name: "relation other kind still applies", relation: true, edit: documentRow("cites"), outcome: OutcomeApplied},
		{name: "relation ignores provision rows", relation: true, edit: matching("replaces", "text_rule/1"), outcome: OutcomeApplied},
		{name: "relation reviewed row ignored", relation: true, edit: func(db *fakeDB, _ *Claim) {
			db.rows = append(db.rows, RelationRow{Kind: "replaces", Method: MethodReviewed, SourceDocumentID: 2, TargetDocumentID: 1})
		}, outcome: OutcomeApplied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := corpus()
			var claim Claim
			if tc.relation {
				claim = relationClaim(t, db)
			} else {
				claim = effectClaim(t, db)
			}
			if tc.edit != nil {
				tc.edit(db, &claim)
			}
			report, err := Check(context.Background(), db, File{Claims: []Claim{claim}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.problem != "" {
				if report.Err() == nil || !strings.Contains(report.Err().Error(), tc.problem) || len(report.Rows) != 0 || len(report.Records) != 0 {
					t.Fatalf("report = %+v", report)
				}
				var invalid *ValidationError
				if !errors.As(report.Err(), &invalid) {
					t.Fatalf("err type %T", report.Err())
				}
				return
			}
			if report.Err() != nil || len(report.Records) != 1 {
				t.Fatalf("report = %+v", report)
			}
			want := Record{ID: claim.ID, Kind: claim.Kind, TargetKey: targetKey, Outcome: tc.outcome, Reason: tc.reason}
			if report.Records[0] != want {
				t.Fatalf("record = %+v, want %+v", report.Records[0], want)
			}
			if applied := tc.outcome == OutcomeApplied; applied != (len(report.Rows) == 1) {
				t.Fatalf("rows = %+v", report.Rows)
			}
		})
	}
}

func TestCheck_AppliedRowsFollowStorageTable(t *testing.T) {
	db := corpus()
	effect, relation := effectClaim(t, db), relationClaim(t, db)
	report, err := Check(context.Background(), db, File{Claims: []Claim{effect, relation}})
	if err != nil || report.Err() != nil || len(report.Rows) != 2 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	e := report.Rows[0]
	if e.Ordinal != 4 || e.Kind != "repeals" || e.Label != "bãi bỏ" || e.Evidence != quote || e.SourceDocumentID != 2 || e.TargetDocumentID != 1 || e.SourceKey != actingKey || e.TargetKey != targetKey || e.SourceNumber != "2/2099/ND-CP" || e.TargetNumber != "1/2099/ND-CP" {
		t.Fatalf("effect row = %+v", e)
	}
	if *e.SourceProvisionID != 21 || *e.TargetProvisionID != 11 || *e.SourceNodeID != "a-khoan" || *e.TargetNodeID != "dieu-5" {
		t.Fatalf("effect row provisions = %+v", e)
	}
	r := report.Rows[1]
	if r.Ordinal != 5 || r.Kind != "replaces" || r.Label != "thay thế" || r.Evidence != quote || r.SourceProvisionID != nil || r.TargetProvisionID != nil || r.SourceNodeID != nil || r.TargetNodeID != nil {
		t.Fatalf("relation row = %+v", r)
	}
}

func TestCheck_LabelsEveryAllowedKind(t *testing.T) {
	want := map[string]string{"repeals": "bãi bỏ", "amends_supplements": "sửa đổi, bổ sung", "replaces": "thay thế", "suspends": "ngưng hiệu lực", "implements": "quy định chi tiết, hướng dẫn thi hành", "guided_by": "được hướng dẫn bởi", "legal_basis": "căn cứ", "cites": "dẫn chiếu", "references": "liên quan"}
	for _, kind := range relationKinds {
		if rawLabel(kind) != want[kind] || want[kind] == "" {
			t.Errorf("label %s = %q", kind, rawLabel(kind))
		}
	}
	if len(want) != len(relationKinds) {
		t.Fatalf("kinds = %v", relationKinds)
	}
}

func TestCheck_CollectsEveryProblemAndKeepsOrder(t *testing.T) {
	db := corpus()
	bad := effectClaim(t, db)
	bad.ID, bad.Quote = "effect-a", "không có trong điều này"
	stale := effectClaim(t, db)
	stale.ID, stale.Contract, stale.TargetAnchor = "effect-b", 9, "dieu-5"
	alsoBad := relationClaim(t, db)
	alsoBad.ID, alsoBad.Quote = "relation-c", "cũng không có ở đây"
	report, err := Check(context.Background(), db, File{Claims: []Claim{bad, stale, alsoBad}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Problems) != 2 || len(report.Records) != 1 || report.Records[0].ID != "effect-b" {
		t.Fatalf("report = %+v", report)
	}
}

func TestCheck_NoClaimsReadsNothing(t *testing.T) {
	report, err := Check(context.Background(), nil, File{Decisions: []ReviewDecision{{ID: "decision-a"}}})
	if err != nil || report.Err() != nil || report.Records == nil || len(report.Records) != 0 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

type failingDB struct{ *fakeDB }

func (failingDB) Documents(context.Context, []string) ([]Document, error) {
	return nil, errors.New("read failed")
}

func TestCheck_ReturnsReadErrors(t *testing.T) {
	db := corpus()
	if _, err := Check(context.Background(), failingDB{db}, File{Claims: []Claim{effectClaim(t, db)}}); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("err = %v", err)
	}
}
