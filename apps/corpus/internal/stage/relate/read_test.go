package relate

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

var testIssuers = map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"}

func TestReadArticles_ReturnsMatchesWithoutDiagram(t *testing.T) {
	nodes := []chunk.Node{
		{NodeID: "article-1", Kind: "dieu", Text: "Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN ngày 30/12/2016. Tạm ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN."},
		{NodeID: "article-2", Kind: "dieu", Text: "Thông tư này có hiệu lực từ ngày 01/01/2099."},
		{NodeID: "article-3", Kind: "dieu", Text: ""},
	}
	report := ReadArticles(nodes, testIssuers, false)
	got := report.Readings
	if len(report.Unread) != 0 {
		t.Fatalf("unread %v", report.Unread)
	}
	if len(got) != 1 || got[0].NodeID != "article-1" || len(got[0].Matches) != 2 {
		t.Fatalf("got %+v", got)
	}
	m := got[0].Matches[0]
	if m.Kind != "suspends" || len(m.Targets) != 1 || m.Targets[0].Number != "39/2016/TT-NHNN" || m.Targets[0].Anchor != "dieu-8/khoan-9" {
		t.Fatalf("match %+v", m)
	}
}

func TestReadArticles_SkipsHeadingOnlyArticlesOfPartialSources(t *testing.T) {
	nodes := []chunk.Node{{NodeID: "a", Kind: "dieu", Label: "Điều 1", Heading: "Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN", Text: "Điều 1. Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN"}}
	if got := ReadArticles(nodes, testIssuers, true); len(got.Readings) != 0 || len(got.Unread) != 1 {
		t.Fatalf("partial source read heading-only article: %+v", got)
	}
}

func TestReadArticles_ReportsArticlesWithUnorderedSource(t *testing.T) {
	const lead = "Điều 1. Sửa đổi, bổ sung một số điều của Thông tư 19/2018/TT-NHNN như sau:"
	const item = "1. Bãi bỏ khoản 2 Điều 5."
	parent := "a"
	nodes := []chunk.Node{
		{NodeID: "a", Kind: "dieu", Text: lead + "\n" + item},
		{NodeID: "c1", ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: item},
		{NodeID: "c2", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "2. Bãi bỏ Điều 6."},
	}
	got := ReadArticles(nodes, map[string]string{"NHNN": "x"}, false)
	if len(got.Readings) != 0 || len(got.Unread) != 1 || got.Unread[0] != "a" {
		t.Fatalf("got %+v", got)
	}
}

func TestReadArticles_EmptyArticleIsUnreadOnlyUnderHeadingGaps(t *testing.T) {
	nodes := []chunk.Node{{NodeID: "a", Kind: "dieu"}}
	if got := ReadArticles(nodes, testIssuers, false); len(got.Unread) != 0 {
		t.Fatalf("complete source: %+v", got)
	}
	if got := ReadArticles(nodes, testIssuers, true); len(got.Unread) != 1 {
		t.Fatalf("heading gap: %+v", got)
	}
}

func TestReadArticles_MatchOrdinalIsSentenceIndex(t *testing.T) {
	nodes := []chunk.Node{{NodeID: "a", Kind: "dieu", Text: "Thông tư này có hiệu lực. Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN."}}
	got := ReadArticles(nodes, testIssuers, false)
	if len(got.Readings) != 1 || got.Readings[0].Matches[0].Ordinal != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestGapKinds_BlockingExcludesHeadingGaps(t *testing.T) {
	for kind, want := range map[string]bool{"needs_ocr": true, "ocr_flagged": true, "tree_headings_partial": false, "tree_headings_only": false, "empty_text": false} {
		if BlockingGap(kind) != want {
			t.Errorf("%s blocking = %v", kind, !want)
		}
	}
	if !IncompleteGap("tree_headings_partial") || IncompleteGap("empty_text") {
		t.Error("incomplete kinds are wrong")
	}
}
