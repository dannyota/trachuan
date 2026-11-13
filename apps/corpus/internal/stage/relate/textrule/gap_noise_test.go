package textrule

import (
	"reflect"
	"testing"
)

func TestParseUnits_BrokenSentenceRejectsOnlyCandidate(t *testing.T) {
	const ownTarget = " của Thông tư 19/2018/TT-NHNN."
	const lead = "Điều 1. Sửa đổi một số điều của Thông tư 19/2018/TT-NHNN như sau:"
	for _, tc := range []struct {
		name    string
		units   []Unit
		anchors []string
		detail  string
	}{
		{name: "broken first item", units: []Unit{{Kind: "dieu", Close: true,
			Text: "1. Bãi bỏ Điều 5\n2. Bãi bỏ Điều 6" + ownTarget,
		}}, anchors: []string{"dieu-6"}, detail: "1. Bãi bỏ Điều 5"},
		{name: "keeps earlier and later rows", units: []Unit{{Kind: "dieu", Close: true,
			Text: "Bãi bỏ Điều 4" + ownTarget + "\n1. Bãi bỏ Điều 5\n2. Bãi bỏ Điều 6" + ownTarget,
		}}, anchors: []string{"dieu-4", "dieu-6"}, detail: "1. Bãi bỏ Điều 5"},
		{name: "broken noncandidate is silent", units: []Unit{{Kind: "dieu", Close: true,
			Text: "1. Nội dung áp dụng\n2. Bãi bỏ Điều 6" + ownTarget,
		}}, anchors: []string{"dieu-6"}},
		{name: "first article sentence is exempt", units: []Unit{{Kind: "dieu", Close: true,
			Text: "Điều 1. Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN\n1. Bãi bỏ Điều 6" + ownTarget,
		}}, anchors: []string{"dieu-5", "dieu-6"}},
		{name: "scoped item", units: []Unit{{Kind: "dieu", Close: true,
			Text: lead + "\n1. Bãi bỏ Điều 5\n2. Bãi bỏ Điều 6.",
		}}, anchors: []string{"dieu-6"}, detail: "1. Bãi bỏ Điều 5"},
		{name: "scoped noncandidate", units: []Unit{{Kind: "dieu", Close: true,
			Text: lead + "\n1. Nội dung áp dụng\n2. Bãi bỏ Điều 6.",
		}}, anchors: []string{"dieu-6"}},
		{name: "clause read unit", units: []Unit{
			{Kind: "dieu", Close: true, Text: lead},
			{Kind: "khoan", FirstItem: true, Close: true,
				Text: "1. Bãi bỏ Điều 5\na) Nội dung áp dụng."},
			{Kind: "khoan", FirstItem: true, Close: true, Text: "2. Bãi bỏ Điều 6."},
		}, anchors: []string{"dieu-6"}, detail: "1. Bãi bỏ Điều 5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches, gaps := ParseUnits(tc.units, testIssuers(), nil)
			var anchors []string
			for _, match := range matches {
				for _, target := range match.Targets {
					anchors = append(anchors, target.Anchor)
				}
			}
			if !reflect.DeepEqual(anchors, tc.anchors) {
				t.Errorf("anchors=%v; want %v", anchors, tc.anchors)
			}
			if tc.detail == "" {
				if len(gaps) != 0 {
					t.Fatalf("unexpected gaps: %+v", gaps)
				}
			} else if len(gaps) != 1 || gaps[0] != (Gap{Name: "relation_text_grammar", Detail: tc.detail}) {
				t.Fatalf("gaps=%+v; want grammar gap for %q", gaps, tc.detail)
			}
		})
	}
}

func TestParse_NoncandidatesRecordNoGap(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{text: "Điều 1. Phạm vi điều chỉnh"},
		{text: "Điều 1. Phạm vi điều chỉnh\n1. Nội dung áp dụng."},
		{text: "Điều 1. Phạm vi điều chỉnh\n1. Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN.", want: 1},
		{text: "Điều 1. Phạm vi điều chỉnh\na) Nội dung áp dụng."},
		{text: "1. Nội dung áp dụng\n2. Nội dung khác."},
		{text: "a) Nội dung áp dụng\nb) Nội dung khác."},
		{text: "Bỏ nội dung áp dụng\n1. Nội dung khác."},
	} {
		t.Run(tc.text, func(t *testing.T) {
			matches, gaps := Parse(tc.text, testIssuers())
			if len(matches) != tc.want || len(gaps) != 0 {
				t.Fatalf("matches=%+v gaps=%+v; want %d matches and no gaps", matches, gaps, tc.want)
			}
		})
	}
}

func TestParse_WordingGapsRequireArticleCandidate(t *testing.T) {
	const repeal = "Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN."
	const definitions = "Điều 3. Giải thích từ ngữ. Các từ ngữ được hiểu như sau:\n1. Nội dung định nghĩa."
	for _, tc := range []struct {
		name, text, gap string
	}{
		{name: "definitions", text: definitions},
		{name: "unclosed definitions quotation", text: definitions + " “Nội dung."},
		{name: "orphan closer", text: "nội dung”"},
		{name: "only quoted verb", text: "“" + repeal},
		{name: "mismatched quoted verb", text: "“Nội dung» " + repeal},
		{name: "candidate before wording", text: repeal + " " + definitions, gap: "relation_text_unquoted_wording"},
		{name: "candidate after wording", text: definitions + " " + repeal, gap: "relation_text_unquoted_wording"},
		{name: "quoted verb after wording", text: definitions + " “" + repeal + "”"},
		{name: "candidate before unclosed quotation", text: repeal + " “Nội dung.", gap: "relation_text_unbalanced_quote"},
		{name: "candidate after orphan closer", text: "nội dung”. " + repeal, gap: "relation_text_unbalanced_quote"},
		{name: "candidate after malformed quotation", text: "“Nội dung».” " + repeal, gap: "relation_text_unbalanced_quote"},
		{name: "special phrase candidate", text: "Bỏ cụm từ “cũ” tại Điều 5 của Thông tư 19/2018/TT-NHNN. " + definitions, gap: "relation_text_unquoted_wording"},
		{name: "bare bo is not candidate", text: "Bỏ nội dung. " + definitions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches, gaps := Parse(tc.text, testIssuers())
			if len(matches) != 0 {
				t.Fatalf("unexpected matches: %+v", matches)
			}
			if tc.gap == "" {
				if len(gaps) != 0 {
					t.Fatalf("unexpected gaps: %+v", gaps)
				}
			} else if len(gaps) != 1 || gaps[0].Name != tc.gap {
				t.Fatalf("gaps=%+v; want one %s", gaps, tc.gap)
			}
		})
	}
}

func TestParseUnits_QuoteGapFindsCandidatesAcrossUnits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		units []Unit
		gap   bool
	}{
		{name: "only quoted candidate across units", units: []Unit{
			{Text: "“Nội dung.", Kind: "dieu", Close: true},
			{Text: "Bãi bỏ Điều 5.”", Kind: "khoan", FirstItem: true, Close: true},
		}},
		{name: "candidate after failing unit", units: []Unit{
			{Text: "nội dung”", Kind: "dieu", Close: true},
			{Text: "1. Bãi bỏ Điều 5.", Kind: "khoan", FirstItem: true, Close: true},
		}, gap: true},
		{name: "article label before candidate", units: []Unit{
			{Text: "Điều 1. Bãi bỏ Điều 5. “Nội dung.", Kind: "dieu", Close: true},
		}, gap: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches, gaps := ParseUnits(tc.units, testIssuers(), nil)
			if len(matches) != 0 || (!tc.gap && len(gaps) != 0) || (tc.gap && (len(gaps) != 1 || gaps[0].Name != "relation_text_unbalanced_quote")) {
				t.Fatalf("matches=%+v gaps=%+v; want quote gap=%v", matches, gaps, tc.gap)
			}
		})
	}
}
