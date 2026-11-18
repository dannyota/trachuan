//go:build integration

package evidence

import (
	"fmt"
	"testing"
)

func (f *releaseFixture) pair(id int64, sourceDoc, targetDoc, sourceKey, targetKey any, kind, method string) {
	f.t.Helper()
	f.exec(`INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal)
 VALUES($1,$2,$3,$4,$5,'1','2',$6,'x','x','e',$7,$8,$9)`,
		id, sourceDoc, targetDoc, sourceKey, targetKey, kind, method, hash64("a"), int32(id))
}

func TestRelationKindDisagreement_AgainstReleaseDatabase(t *testing.T) {
	f := newReleaseFixture(t)
	for i, key := range []string{"vn:src", "vn:target", "vn:other", "vn:norows", "vn:base", "vn:gapped", "vn:heading"} {
		f.document(int64(i+1), key, fmt.Sprintf("%d/2099/TT-NHNN", i+1), "T", "in_force", "source_tree", "")
	}
	for _, id := range []int64{1, 2, 3, 6, 7} {
		f.provision(id*10, id, nil, "dieu", "dieu-1", "Điều 1", 1)
	}
	f.gap(1, "build", "baseline_served", "vn:base")
	f.gap(2, "relate", "relation_text_unlinked_target", "vn:gapped")
	f.gap(3, "extract", "tree_headings_partial", "vn:heading")
	f.pair(1, 1, 2, "vn:src", "vn:target", "repeals", "vbpl_diagram")
	f.pair(2, 1, 2, "vn:src", "vn:target", "amends_supplements", "text_rule/1")
	f.pair(3, 1, 3, "vn:src", "vn:other", "amends_supplements", "vbpl_references,vbpl_diagram")
	f.pair(4, 1, 3, "vn:src", "vn:other", "suspends", "text_rule/1")
	f.pair(5, 1, 3, "vn:src", "vn:other", "repeals", "reviewed")
	f.pair(6, 4, 2, "vn:norows", "vn:target", "cites", "vbpl_diagram")
	f.pair(7, 4, 2, "vn:norows", "vn:target", "repeals", "text_rule/1")
	f.pair(8, 5, 2, "vn:base", "vn:target", "cites", "vbpl_diagram")
	f.pair(9, 5, 2, "vn:base", "vn:target", "repeals", "text_rule/1")
	f.pair(10, 6, 2, "vn:gapped", "vn:target", "cites", "vbpl_diagram")
	f.pair(11, 6, 2, "vn:gapped", "vn:target", "repeals", "text_rule/1")
	f.pair(12, 7, 2, "vn:heading", "vn:target", "cites", "vbpl_diagram")
	f.pair(13, 7, 2, "vn:heading", "vn:target", "repeals", "text_rule/1")
	f.pair(14, nil, 2, "vn:ghost", "vn:target", "cites", "vbpl_diagram")
	f.pair(15, nil, 2, "vn:ghost", "vn:target", "repeals", "text_rule/1")
	f.pair(16, 1, 2, "vn:src", nil, "cites", "vbpl_diagram")
	f.pair(17, 1, 2, "vn:src", nil, "repeals", "text_rule/1")

	var pairs []pairRelation
	if err := (sqlRelease{f.pool}).PairRelations(f.ctx, func(p pairRelation) error { pairs = append(pairs, p); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, p := range pairs {
		if p.Source == "vn:ghost" || p.Target == "" {
			t.Fatalf("filter let through %+v", p)
		}
	}
	if len(pairs) != 13 {
		t.Fatalf("%d pairs: %+v", len(pairs), pairs)
	}

	obs, err := kindDetector{}.Detect(f.ctx, Inputs{Release: f.pool})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, fired := range obs.Findings {
		got[fired.Subject.Fields[0]+"|"+fired.Subject.Fields[2]] = obs.ObservedCompletely(fired.Subject)
	}
	want := map[string]bool{"vn:src|amends_supplements": true, "vn:norows|repeals": false, "vn:base|repeals": false, "vn:gapped|repeals": false, "vn:heading|repeals": false}
	if len(got) != len(want) || obs.Excluded != 1 {
		t.Fatalf("findings %v excluded %d", got, obs.Excluded)
	}
	for key, observed := range want {
		if v, ok := got[key]; !ok || v != observed {
			t.Errorf("%s: present %v observed %v, want observed %v", key, ok, v, observed)
		}
	}
	if obs.Observed != 3 || obs.NotObservable != 4 {
		t.Fatalf("observed %d not observable %d", obs.Observed, obs.NotObservable)
	}
}
