//go:build integration

package retrieval

import (
	"context"
	"testing"
	"time"
)

func contextFixture(t *testing.T) *Engine {
	t.Helper()
	db, e, _ := fixture(t)
	_, err := db.Exec(context.Background(), `UPDATE document SET class='relation_context', number_norm='50/2024/TT-NHNN' WHERE id IN (1,2)`)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func checkContext(t *testing.T, a Arms, wantPrimary, wantNonCurrent bool) {
	t.Helper()
	for _, tc := range []struct {
		name  string
		group ArmGroup
		want  bool
	}{{"primary", a.Primary, wantPrimary}, {"non-current", a.NonCurrent, wantNonCurrent}} {
		for _, arm := range [][]Candidate{tc.group.Dense, tc.group.Lexical} {
			found := false
			for _, c := range arm {
				if c.Identity.DocumentKey == "vn:1" || c.Identity.DocumentKey == "vn:2" {
					found = true
				}
			}
			if found != tc.want {
				t.Errorf("%s context found = %v, want %v: %+v", tc.name, found, tc.want, arm)
			}
		}
	}
}

func TestCollect_ContextDocumentExcludedByDefault(t *testing.T) {
	e := contextFixture(t)
	no := false
	for _, mode := range []Mode{"exact", "ann"} {
		a := collectFixture(t, e, Request{Query: "ngân hàng hết hạn", Mode: mode, InForceOnly: &no}, Depths{10, 10})
		checkContext(t, a, false, false)
	}
}

func TestCollect_ContextDocumentAdmittedByNumberReference(t *testing.T) {
	e := contextFixture(t)
	no := false
	for _, mode := range []Mode{"exact", "ann"} {
		a := collectFixture(t, e, Request{Query: "ngân hàng hết hạn 50 / 2024 / tt - nhnn", Mode: mode, InForceOnly: &no}, Depths{10, 10})
		checkContext(t, a, true, true)
	}
}

func TestCollect_ContextRuleInAsOfMode(t *testing.T) {
	e := contextFixture(t)
	no := false
	for _, mode := range []Mode{"exact", "ann"} {
		for _, tc := range []struct {
			query string
			want  bool
		}{{"ngân hàng hết hạn", false}, {"ngân hàng hết hạn 50/2024/TT-NHNN", true}} {
			a := collectFixture(t, e, Request{Query: tc.query, Mode: mode, InForceOnly: &no, AsOf: &Date{2022, time.June, 1}}, Depths{10, 10})
			checkContext(t, a, tc.want, tc.want)
		}
	}
}

func TestCollect_ANNPoolOfContextFallsBackToExact(t *testing.T) {
	e := contextFixture(t)
	v := make([]float32, 1024)
	v[1] = 1
	if _, err := e.db.Exec(context.Background(), `UPDATE chunk SET embedding=$1::vector WHERE id=5`, denseVector(v)); err != nil {
		t.Fatal(err)
	}
	e.release.RetrievalRecipe.ANN.PoolMin = 2
	e.release.RetrievalRecipe.ANN.PoolMultiplier = 1
	a := collectFixture(t, e, Request{Query: "ngân hàng"}, Depths{2, 2})
	if !a.Primary.Fallback || len(a.Primary.RawANN) != 0 || len(a.Primary.Dense) != 1 || a.Primary.Dense[0].Identity.DocumentKey != "vn:3" {
		t.Fatalf("primary = %+v", a.Primary)
	}
}
