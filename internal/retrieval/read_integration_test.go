//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func readFixture(t *testing.T) (*pgxpool.Pool, *Engine) {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testpg.NewDatabase(t, "vector", "ltree"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = corpusdb.Apply(ctx, db, 1024); err != nil {
		t.Fatal(err)
	}
	readExec(t, db, `INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) SELECT i,'vn','vn:'||i,'N'||i,'N'||i,'title','law','issuer','primary','vbpl',false,'2020-01-01','','','','','','in_force','source_status',true,false,'indexed',$1,'source_tree',true FROM generate_series(1,3) i`, strings.Repeat("a", 64))
	nodes := []struct {
		id, doc           int
		parent            any
		kind, label, text string
	}{
		{1, 1, nil, "phan", "Phần I", "Phần I"},
		{2, 1, 1, "chuong", "Chương I", "Chương I"},
		{3, 1, 2, "muc", "Mục 1", "Mục 1"},
		{4, 1, 3, "dieu", "Điều 1", "Điều kiện áp dụng:"},
		{5, 1, 4, "khoan", "1", "Nghĩa vụ thứ nhất:"},
		{6, 1, 5, "diem", "a", "a) Thực hiện nghĩa vụ."},
		{7, 1, 4, "khoan", "2", "2. Nghĩa vụ thứ hai."},
		{8, 1, 4, "khoan", "3", "3. Nghĩa vụ thứ ba."},
		{9, 1, 3, "dieu", "Điều 2", strings.Repeat("á", 40000)},
		{10, 2, nil, "other", "Phụ lục", "Nội dung phụ lục."},
		{11, 2, 10, "khoan", "1", "Nội dung chi tiết."},
	}
	for _, n := range nodes {
		readExec(t, db, `INSERT INTO provision(id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES($1,$2,$3,$4,$5,$6,'heading',$8,$8,$4::text::ltree,$7,$4)`, n.id, n.doc, n.parent, fmt.Sprintf("p%d", n.id), n.kind, n.label, n.text, n.id)
	}
	return db, &Engine{db: db, release: Release{Rel: "20991110_1", RetrievalRecipe: Recipe2()}, options: fixtureOptions()}
}

func readExec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func readItems(t *testing.T, e *Engine, items ...ReadItem) ReadResult {
	t.Helper()
	r, err := e.Read(context.Background(), ReadRequest{Release: e.release.Rel, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if r.Release != e.release.Rel || len(r.Items) != len(items) {
		t.Fatal("wrong result scope")
	}
	return r
}

func TestRead_DocumentOutline(t *testing.T) {
	_, e := readFixture(t)
	r := readItems(t, e, ReadItem{Handle: "vn:1"}, ReadItem{Handle: "vn:2"})
	a := r.Items[0]
	if a.Document == nil || a.Document.Number != "N1" || a.Document.TextSource != "source_tree" || !a.Document.TextComplete || a.Document.Status.Class != "in_force" || !a.Document.Status.Current || !a.Complete || a.Next != "" {
		t.Fatalf("outline metadata: %+v", a)
	}
	want := []string{"p1", "p2", "p3", "p4", "p9"}
	if len(a.Units) != len(want) {
		t.Fatalf("outline units: %d", len(a.Units))
	}
	for i, u := range a.Units {
		if u.Kind != "outline" || u.Anchor != want[i] || u.Label == "" || u.Heading != "heading" || u.Size == 0 || u.Text != "" {
			t.Fatalf("outline unit: %+v", u)
		}
	}
	if a.Units[4].Size != 40000 {
		t.Fatalf("article size %d", a.Units[4].Size)
	}
	b := r.Items[1]
	if b.Document == nil || len(b.Units) != 1 || b.Units[0].Anchor != "p10" {
		t.Fatalf("fallback outline: %+v", b)
	}
}

func TestRead_ProvisionWithLeads(t *testing.T) {
	_, e := readFixture(t)
	r := readItems(t, e, ReadItem{Handle: "vn:1#p5"}).Items[0]
	if !r.Complete || r.Next != "" || len(r.Units) != 2 {
		t.Fatalf("provision: %+v", r)
	}
	if r.Units[0].Kind != "lead" || r.Units[0].Text != "Điều kiện áp dụng:" || r.Units[1].Text != "Nghĩa vụ thứ nhất:\na) Thực hiện nghĩa vụ." || r.Units[1].Citation != "Điều 1, 1" {
		t.Fatalf("lead or subtree: %+v", r.Units)
	}
}

func TestRead_LabelOnlyLeadOmitted(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `UPDATE provision SET text=$1,label=$2 WHERE id=4`, "  ĐIỀU 1.:)  ", "Điều 1")
	r := readItems(t, e, ReadItem{Handle: "vn:1#p5"}).Items[0]
	if !r.Complete || len(r.Units) != 1 || r.Units[0].Kind != "provision" {
		t.Fatalf("label lead: %+v", r)
	}
}

func TestRead_AmbiguousTreeIncomplete(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `UPDATE provision SET text=$1 WHERE id=4`, "Nghĩa vụ thứ nhất:\na) Thực hiện nghĩa vụ.")
	// Only one of three children occurs, so the ancestor rendering is ambiguous.
	for _, handle := range []string{"vn:1#p5", "vn:1#p4"} {
		r := readItems(t, e, ReadItem{Handle: handle}).Items[0]
		if r.Complete || len(r.Gaps) == 0 || r.Gaps[0].Kind != "ambiguous_tree_text" {
			t.Fatalf("ambiguous: %+v", r)
		}
		for _, u := range r.Units {
			if u.Kind == "lead" {
				t.Fatal("ambiguous lead returned")
			}
		}
	}
}

func TestRead_RangeInclusiveOwnTextOnce(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `UPDATE provision SET text=$1 WHERE id=5`, "Nghĩa vụ thứ nhất:\na) Thực hiện nghĩa vụ.")
	r := readItems(t, e, ReadItem{Handle: "vn:1", From: "p5", To: "p8"}).Items[0]
	want := []string{"Điều kiện áp dụng:", "Nghĩa vụ thứ nhất:", "a) Thực hiện nghĩa vụ.", "2. Nghĩa vụ thứ hai.", "3. Nghĩa vụ thứ ba."}
	if !r.Complete || r.Next != "" || len(r.Units) != len(want) {
		t.Fatalf("range: %+v", r)
	}
	seen := map[string]bool{}
	for i, u := range r.Units {
		if u.Text != want[i] || seen[u.Anchor] {
			t.Fatalf("range unit: %+v", u)
		}
		seen[u.Anchor] = true
	}
}

func TestRead_InvalidRange(t *testing.T) {
	_, e := readFixture(t)
	for _, item := range []ReadItem{{Handle: "vn:1", From: "p8", To: "p5"}, {Handle: "vn:1", From: "p5", To: "p10"}, {Handle: "vn:1", From: "p5", To: "vn:2#p10"}} {
		if r := readItems(t, e, item).Items[0]; r.Error != "invalid_range" {
			t.Fatalf("range error: %+v", r)
		}
	}
}

func TestRead_LongProvisionParts(t *testing.T) {
	_, e := readFixture(t)
	item := ReadItem{Handle: "vn:1#p9"}
	var text strings.Builder
	var parts []Unit
	pages := 0
	for {
		r := readItems(t, e, item).Items[0]
		pages++
		if r.Error != "" {
			t.Fatal(r.Error)
		}
		for _, u := range r.Units {
			if utf8.RuneCountInString(u.Text) > PartSize || u.Text == "" {
				t.Fatal("bad part length")
			}
			text.WriteString(u.Text)
			parts = append(parts, u)
		}
		if r.Next == "" {
			if !r.Complete {
				t.Fatal("last page incomplete")
			}
			break
		}
		if r.Complete || pages > 10 {
			t.Fatal("bad continuation")
		}
		item = ReadItem{Next: r.Next}
	}
	if pages < 2 || text.String() != strings.Repeat("á", 40000) {
		t.Fatal("parts lost text or failed to page")
	}
	for i, p := range parts {
		if p.Part != i+1 || p.Parts != len(parts) {
			t.Fatal("bad part numbering")
		}
	}
}

func TestRead_CallCapNotRead(t *testing.T) {
	_, e := readFixture(t)
	item := ReadItem{Handle: "vn:1#p9"}
	r := readItems(t, e, item, item)
	if r.Items[1].NotRead != "call_cap" || len(r.Items[1].Units) != 0 || r.Items[1].Next == "" {
		t.Fatalf("cap: %+v", r.Items[1])
	}
	resumed := readItems(t, e, ReadItem{Next: r.Items[1].Next}).Items[0]
	alone := readItems(t, e, item).Items[0]
	if !reflect.DeepEqual(resumed, alone) {
		t.Fatal("deferred item changed")
	}
}

func TestRead_FirstItemAlwaysReturnsUnit(t *testing.T) {
	_, e := readFixture(t)
	r := readItems(t, e, ReadItem{Handle: "vn:1"}).Items[0]
	cost := readSize(r.Document)
	for _, unit := range r.Units {
		cost += readSize(unit)
	}
	if r.Document == nil || len(r.Units) == 0 || r.NotRead != "" || cost > ReadCallCap {
		t.Fatal("first item violated progress or the call cap")
	}
}

func TestRead_OversizedDocumentTitleIsTerminal(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `UPDATE document SET title=$1 WHERE id=1`, strings.Repeat("x", ReadCallCap))
	r := readItems(t, e, ReadItem{Handle: "vn:1"}).Items[0]
	if r.Error != "unit_too_large" || r.Next != "" || r.Document != nil || len(r.Units) != 0 || r.Complete {
		t.Fatal("oversized document escaped the call cap")
	}
}

func TestRead_DuplicateItemsIndependent(t *testing.T) {
	_, e := readFixture(t)
	item := ReadItem{Handle: "vn:1#p5"}
	r := readItems(t, e, item, item)
	if !reflect.DeepEqual(r.Items[0], r.Items[1]) {
		t.Fatal("duplicate items differed")
	}
}

func TestRead_RelationsPaged(t *testing.T) {
	db, e := readFixture(t)
	kinds := []string{"cites", "repeals", "implements"}
	for i := 1; i <= 45; i++ {
		source, target, sk, tk, number := 1, 2, "vn:1", "vn:2", fmt.Sprintf("N%02d", i)
		sn, tn := "one", number
		if i%2 == 0 {
			source, target, sk, tk, sn, tn = 2, 1, "vn:2", "vn:1", number, "one"
		}
		evidence := "short evidence"
		if i%2 == 1 {
			evidence = strings.Repeat("đ", 1100)
		}
		readExec(t, db, `INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'',$9,$10,'source',$11,$12)`, i, source, target, sk, tk, sn, tn, kinds[i%3], fmt.Sprintf("raw %d", i), evidence, strings.Repeat("b", 64), i)
	}
	var want []int
	for i := 1; i <= 45; i++ {
		want = append(want, i)
	}
	sort.Slice(want, func(a, b int) bool {
		i, j := want[a], want[b]
		ki, kj := pointerKindRank(kinds[i%3]), pointerKindRank(kinds[j%3])
		if ki != kj {
			return ki < kj
		}
		if i%2 != j%2 {
			return i%2 == 0
		}
		return i < j
	})
	item := ReadItem{Handle: "vn:1", Relations: true}
	var got []Unit
	for _, count := range []int{20, 20, 5} {
		r := readItems(t, e, item).Items[0]
		if len(r.Units) != count {
			t.Fatalf("relation page length %d, want %d", len(r.Units), count)
		}
		got = append(got, r.Units...)
		if count == 5 {
			if r.Next != "" {
				t.Fatal("extra relation page")
			}
		} else if r.Next == "" || r.Complete {
			t.Fatal("missing relation continuation")
		}
		item = ReadItem{Next: r.Next}
	}
	for pos, u := range got {
		i := want[pos]
		r := u.Relation
		if u.Kind != "relation" || r == nil || r.RawLabel != fmt.Sprintf("raw %d", i) || r.Number != fmt.Sprintf("N%02d", i) || r.Key != "vn:2" || !r.InRelease {
			t.Fatalf("relation at %d: %+v", pos, r)
		}
		if i%2 == 1 {
			if utf8.RuneCountInString(r.Evidence) != 1000 || r.EvidenceComplete {
				t.Fatal("evidence cap")
			}
		} else if r.Evidence != "short evidence" || !r.EvidenceComplete {
			t.Fatal("short evidence")
		}
	}
}

func TestRead_NumberLookup(t *testing.T) {
	db, e := readFixture(t)
	byNumber := readItems(t, e, ReadItem{Number: " n1 "}).Items[0]
	byHandle := readItems(t, e, ReadItem{Handle: "vn:1"}).Items[0]
	if !reflect.DeepEqual(byNumber, byHandle) {
		t.Fatal("number lookup differs")
	}
	readExec(t, db, `UPDATE document SET number_norm='N1' WHERE id=2`)
	if r := readItems(t, e, ReadItem{Number: "N1"}).Items[0]; r.Error != "ambiguous_number" {
		t.Fatal("duplicate number accepted")
	}
	if r := readItems(t, e, ReadItem{Number: "missing"}).Items[0]; r.Error != "not_found" {
		t.Fatal("missing number accepted")
	}
}

func TestRead_ItemErrorsDoNotFailCall(t *testing.T) {
	_, e := readFixture(t)
	r := readItems(t, e, ReadItem{Handle: "vn:missing"}, ReadItem{Handle: "vn:1", Number: "N1"}, ReadItem{Handle: "vn:1#p5"})
	if r.Items[0].Error != "not_found" || r.Items[1].Error != "invalid_item" || !r.Items[2].Complete {
		t.Fatalf("item errors: %+v", r)
	}
	expired, err := e.Read(context.Background(), ReadRequest{Release: "old", Items: []ReadItem{{Handle: "vn:1"}}})
	if err != nil || expired.Release != e.release.Rel || expired.Items[0].Error != "release_expired" {
		t.Fatalf("release: %+v %v", expired, err)
	}
}

func TestRead_NulOrInvalidUTF8IsItemErrorMixedBatch(t *testing.T) {
	_, e := readFixture(t)
	valid := ReadItem{Handle: "vn:1#p5"}
	want := readItems(t, e, valid).Items[0]
	for name, value := range map[string]string{"nul": "\x00", "invalid_utf8": "\xff"} {
		for field, item := range readTextItems(value) {
			t.Run(name+"/"+field, func(t *testing.T) {
				got := readItems(t, e, item, valid, ReadItem{Next: readTestNext(item)})
				if got.Items[0].Error != "invalid_item" || got.Items[2].Error != "invalid_next" || !reflect.DeepEqual(got.Items[1], want) {
					t.Fatalf("invalid input failed a valid batch item: %+v", got)
				}
			})
		}
	}
}

func TestRead_HostileNumberIsItemErrorMixedBatch(t *testing.T) {
	_, e := readFixture(t)
	item := ReadItem{Number: "1/" + strings.Repeat(" ", 60000) + "2026/QH"}
	valid := ReadItem{Number: "N1"}
	want := readItems(t, e, valid).Items[0]
	got := readItems(t, e, item, valid, ReadItem{Next: readTestNext(item)})
	if got.Items[0].Error != "invalid_item" || got.Items[2].Error != "invalid_next" || !reflect.DeepEqual(got.Items[1], want) {
		t.Fatalf("hostile number failed a valid batch item: %+v", got)
	}
}

func TestRead_TamperedNextIsItemError(t *testing.T) {
	_, e := readFixture(t)
	for _, tc := range []struct {
		c    cursor
		want string
	}{
		{cursor{V: 1, Release: e.release.Rel, Item: ReadItem{Handle: "vn:1#p5"}, Unit: 99}, "invalid_next"},
		{cursor{V: 1, Release: e.release.Rel, Item: ReadItem{Handle: "vn:1#p5"}, Part: 99}, "invalid_next"},
		{cursor{V: 1, Release: e.release.Rel, Item: ReadItem{Handle: "vn:1", From: "p5"}}, "invalid_next"},
		{cursor{V: 1, Release: e.release.Rel, Item: ReadItem{Number: "N1", Relations: true}}, "invalid_next"},
		{cursor{V: 1, Release: "other", Item: ReadItem{Handle: "vn:1"}}, "release_expired"},
	} {
		if r := readItems(t, e, ReadItem{Next: encodeNext(tc.c)}).Items[0]; r.Error != tc.want {
			t.Fatalf("cursor error %q, want %q", r.Error, tc.want)
		}
	}
	if r := readItems(t, e, ReadItem{Next: "bad"}).Items[0]; r.Error != "invalid_next" {
		t.Fatal("malformed token accepted")
	}
}

func TestRead_TooManyItems(t *testing.T) {
	_, e := readFixture(t)
	_, err := e.Read(context.Background(), ReadRequest{Release: e.release.Rel, Items: make([]ReadItem, 9)})
	if !errors.Is(err, ErrTooManyItems) {
		t.Fatalf("error %v", err)
	}
}

func TestRead_ResultSameWhateverElseCallReads(t *testing.T) {
	_, e := readFixture(t)
	item := ReadItem{Handle: "vn:1#p9"}
	alone := readItems(t, e, item).Items[0]
	batch := readItems(t, e, ReadItem{Handle: "vn:2"}, item).Items[1]
	if !reflect.DeepEqual(alone, batch) {
		t.Fatal("item boundaries depend on batch")
	}
	next := ReadItem{Next: alone.Next}
	a := readItems(t, e, next).Items[0]
	b := readItems(t, e, ReadItem{Handle: "vn:1#p5"}, next).Items[1]
	if !reflect.DeepEqual(a, b) {
		t.Fatal("continuation changed with other items")
	}
}

func TestRead_ContinuationSequenceSameAcrossBudgets(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `UPDATE provision SET text=$1 WHERE id=9`, strings.Repeat("a", 100000))
	readExec(t, db, `UPDATE provision SET text=$1 WHERE id=8`, strings.Repeat("b", 16000))
	initial := readItems(t, e, ReadItem{Handle: "vn:1#p9"}).Items[0]
	if initial.Next == "" {
		t.Fatal("missing initial continuation")
	}
	collect := func(prefix bool) ([]Unit, int) {
		t.Helper()
		item := ReadItem{Next: initial.Next}
		var units []Unit
		firstCount := 0
		for page := 0; page < 20; page++ {
			items := []ReadItem{item}
			if prefix {
				items = append([]ReadItem{{Handle: "vn:1#p8"}}, item)
			}
			result := readItems(t, e, items...).Items[len(items)-1]
			if result.Error != "" {
				t.Fatal(result.Error)
			}
			if page == 0 {
				firstCount = len(result.Units)
			}
			for _, unit := range result.Units {
				if readSize(unit) > PartSize {
					t.Fatal("oversized part")
				}
			}
			units = append(units, result.Units...)
			if result.Next == "" {
				if !result.Complete {
					t.Fatal("incomplete chain")
				}
				return units, firstCount
			}
			item = ReadItem{Next: result.Next}
		}
		t.Fatal("chain did not terminate")
		return nil, 0
	}
	alone, aloneCount := collect(false)
	batch, batchCount := collect(true)
	if aloneCount <= batchCount || batchCount == 0 || !reflect.DeepEqual(alone, batch) {
		t.Fatal("continuation sequence changed across a cap boundary")
	}
}

func TestRead_UnitTooLargeIsItemError(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `UPDATE provision SET heading=$1 WHERE id=9`, strings.Repeat("h", 20000))
	item := ReadItem{Handle: "vn:1#p9"}
	next := encodeNext(cursor{V: 1, Release: e.release.Rel, Item: item})
	result := readItems(t, e, item, ReadItem{Next: next}, ReadItem{Handle: "vn:2"})
	for _, page := range result.Items[:2] {
		if page.Error != "unit_too_large" || page.Next != "" || page.Complete || len(page.Units) != 0 {
			t.Fatalf("expected terminal unit error: %+v", page)
		}
	}
	if !result.Items[2].Complete || result.Items[2].Error != "" {
		t.Fatal("unit error failed another item")
	}
}

func TestRead_ItemDocAndAnchorsIntegration(t *testing.T) {
	_, e := readFixture(t)
	for _, item := range []ReadItem{{Handle: "vn:1"}, {Number: "N1"}, {Handle: "vn:1#p5"}, {Handle: "vn:1", From: "p8", To: "p5"}, {Handle: "vn:1#missing"}} {
		result := readItems(t, e, item).Items[0]
		b, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"doc":"vn:1"`) || strings.Contains(string(b), `"handle"`) {
			t.Fatalf("item JSON: %s", b)
		}
	}
	page := readItems(t, e, ReadItem{Handle: "vn:1#p9"}).Items[0]
	if page.Next == "" {
		t.Fatal("missing continuation")
	}
	page = readItems(t, e, ReadItem{Next: page.Next}).Items[0]
	b, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"doc":"vn:1"`) || !strings.Contains(string(b), `"anchor":"p9"`) {
		t.Fatalf("continuation JSON: %s", b)
	}
}

func TestRead_RelationKeyIsResolvedKey(t *testing.T) {
	db, e := readFixture(t)
	insertDocumentRelation(t, db, 1, 1, 2, "vn:1", nil, "one", "two", "cites", strings.Repeat("a", 64), 1)
	result := readItems(t, e, ReadItem{Handle: "vn:1", Relations: true}).Items[0]
	if result.Doc != "vn:1" || len(result.Units) != 1 || result.Units[0].Relation == nil || result.Units[0].Relation.Key != "vn:2" || !result.Units[0].Relation.InRelease {
		t.Fatalf("relation: %+v", result)
	}
}
