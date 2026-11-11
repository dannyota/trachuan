package build

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuleC_UnresolvedActorArticleMakesNoConflict(t *testing.T) {
	for _, article := range []bool{false, true} {
		a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
		b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
		if article {
			b.article = testArticle("1/2/2021", "Nghị định này hết hiệu lực theo quy định khác.")
		}
		v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 3}}, nil)
		if v.documents["a"].values.Conflict || !hasReview(v, "replaces_unverified") {
			t.Fatalf("article=%v: %+v", article, v)
		}
	}
}

func TestRuleC_UsableArticleSilentOnTargetSetsConflict(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", "")
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 3}}, nil)
	if !v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}

func TestRuleC_UsableArticlePartialMentionNoConflict(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", "Nghị định này bãi bỏ Điều 3 của Nghị định số 1/2020/NĐ-CP.")
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 3}}, nil)
	if v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}

func TestReview_ReplacesUnverifiedEntry(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.quote = strings.Repeat("á", 450) + " Nghị định số 1/2020/NĐ-CP."
	b.article = testArticle("1/2/2021", "Nghị định này hết hiệu lực theo quy định khác.")
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 3}}, nil)
	data, err := encodeValidityReview(v.reviews)
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		Entries []struct {
			Kind       string         `json:"kind"`
			Key        string         `json:"key"`
			TargetKey  string         `json:"target_key"`
			RelationID int64          `json:"relation_id"`
			Sentences  []string       `json:"sentences"`
			Proposed   map[string]any `json:"proposed"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	for _, entry := range artifact.Entries {
		if entry.Kind == "replaces_unverified" {
			if entry.Key != "b" || entry.TargetKey != "a" || entry.RelationID != 3 || len(entry.Sentences) != 1 || entry.Sentences[0] != b.quote || entry.Proposed["key"] != "a" || entry.Proposed["eligible"] != nil {
				t.Fatalf("entry=%+v", entry)
			}
			counts := map[string]int64{}
			addValidityCounts(counts, v.counts)
			if counts["validity_review_replaces_unverified"] != 1 {
				t.Fatal(counts)
			}
			return
		}
	}
	t.Fatalf("missing review: %s", data)
}

func TestRuleC_WholeMentionWithoutEventNoConflict(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	a.issuer = "Bộ khác"
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 3}}, nil)
	if v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}
