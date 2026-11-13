package textrule

import "testing"

func TestParse_RejectsInsertedLocatorLabels(t *testing.T) {
	// Relations-text version 2 adds inserted labels with its own recipe change.
	for _, locator := range []string{"khoản 2a Điều 5", "khoản 2đ Điều 5", "điểm A1 khoản 2 Điều 5", "điểm a1 Điều 5", "điểm a Điều 5a"} {
		matches, gaps := Parse("Bãi bỏ "+locator+" của Thông tư 39/2016/TT-NHNN.", testIssuers())
		if len(matches) != 0 || len(gaps) != 1 || gaps[0].Name != "relation_text_grammar" {
			t.Errorf("%s: matches=%+v gaps=%+v", locator, matches, gaps)
		}
	}
}

func TestParse_RejectsMalformedInsertedLocators(t *testing.T) {
	for _, locator := range []string{"khoản 2ab Điều 5", "điểm a123 Điều 5", "điểm aa1 Điều 5", "khoản a2 Điều 5"} {
		matches, gaps := Parse("Bãi bỏ "+locator+" của Thông tư 39/2016/TT-NHNN.", testIssuers())
		if len(matches) != 0 || len(gaps) != 1 || gaps[0].Name != "relation_text_grammar" {
			t.Errorf("%s: matches=%+v gaps=%+v", locator, matches, gaps)
		}
	}
}
