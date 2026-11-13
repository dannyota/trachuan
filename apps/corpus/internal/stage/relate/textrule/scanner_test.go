package textrule

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func testCursor(text string) cursor {
	return cursor{text: text, tokens: tokenize(text, make([]bool, len(text)))}
}

func TestQuoteMask_NestingAndProvisionScope(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		balanced   bool
		quoted     []string
	}{
		{"curly and guillemets", "“Nội dung «bên trong» còn trong” bênngoài", true, []string{"nội", "dung", "bên", "trong", "còn", "trong"}},
		{"straight nested", "\"Nội dung 'bên trong' còn trong\" bênngoài", true, []string{"nội", "dung", "bên", "trong", "còn", "trong"}},
		{"across sentences", "“Mở. Bãi bỏ Điều 3. Vẫn trong” bênngoài", true, []string{"mở", "bãi", "bỏ", "điều", "3", "vẫn", "trong"}},
		{"wrong closer", "“Nội dung»", false, nil},
		{"missing outer closer", "“Nội dung «bên trong»", false, nil},
		{"orphan closer", "Nội dung”", false, nil},
		{"unclosed apostrophe", "'Nội dung", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mask, balanced := quoteMask(tc.text)
			if balanced != tc.balanced {
				t.Fatalf("balanced = %v, want %v", balanced, tc.balanced)
			}
			if !balanced {
				return
			}
			var words []string
			for _, sentence := range (reading{text: tc.text, mask: mask}).scan() {
				for _, token := range sentence.tokens {
					if token.quoted && numberWord(token.word) {
						words = append(words, token.word)
					}
				}
			}
			if !reflect.DeepEqual(words, tc.quoted) {
				t.Fatalf("quoted words = %v, want %v", words, tc.quoted)
			}
		})
	}
}

func TestSentences_ListMarkersAndNumberStops(t *testing.T) {
	text := "1. Bãi bỏ Điều 3 của Nghị định 1.234/2020/NĐ-CP. 2. Thay thế Điều 4 của Thông tư 39/2016/TT-NHNN."
	mask, _ := quoteMask(text)
	parts := (reading{text: text, mask: mask}).scan()
	if len(parts) != 2 || parts[0].text != "1. Bãi bỏ Điều 3 của Nghị định 1.234/2020/NĐ-CP." || parts[1].text != "2. Thay thế Điều 4 của Thông tư 39/2016/TT-NHNN." {
		t.Fatalf("sentences = %#v", parts)
	}
	matches, gaps := Parse(text, testIssuers())
	if len(matches) != 2 || len(gaps) != 0 || matches[0].Targets[0].Number != "1.234/2020/NĐ-CP" || matches[1].Targets[0].Number != "39/2016/TT-NHNN" {
		t.Fatalf("matches = %#v, gaps = %#v", matches, gaps)
	}
}

func TestNumber_TokenBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text, want, rest string
		ok               bool
	}{
		{"39 / 2016 / TT - NHNN ngày", "39 / 2016 / TT - NHNN", "ngày", true},
		{"39/2016/TT-NHNN.", "39/2016/TT-NHNN", ".", true},
		{"1.234/2020/NĐ-CP.", "1.234/2020/NĐ-CP", ".", true},
		{"01/QĐ-UBND,", "01/QĐ-UBND", ",", true},
		{"01/2020/TTLT-BTP-CP;", "01/2020/TTLT-BTP-CP", ";", true},
		{"39//2016/TT-NHNN", "", "", false},
		{"39/2016/TT-", "", "", false},
		{"39/2016/TT/NHNN", "", "", false},
		{"30/12/2016", "", "", false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			c := testCursor(tc.text)
			number, ok := c.number()
			if ok != tc.ok || number != tc.want {
				t.Fatalf("number = %q, %v; want %q, %v", number, ok, tc.want, tc.ok)
			}
			if !ok {
				if c.pos != 0 {
					t.Fatal("failed number consumed tokens")
				}
				return
			}
			if c.pos >= len(c.tokens) || c.tokens[c.pos].word != tc.rest {
				t.Fatalf("next token should be %q, cursor = %#v", tc.rest, c)
			}
		})
	}
}

func TestNormalize_AllDigitPlanesAndNFC(t *testing.T) {
	if got := normalize("  BÃI\tBỎ\nĐiều 𝟛 của ٣٩/२०१६/TT-NHNN. "); got != "BÃI BỎ Điều 3 của 39/2016/TT-NHNN." {
		t.Fatalf("normalize = %q", got)
	}
}

func TestParse_ClosedVerbTable(t *testing.T) {
	for kind, verbs := range map[string][]string{
		"amends_supplements": {"Sửa đổi", "Bổ sung", "Sửa đổi và bổ sung"},
		"repeals":            {"Bãi bỏ", "Hủy bỏ", "Bãi bỏ, hủy bỏ"},
		"replaces":           {"Thay thế"},
		"suspends":           {"Ngưng hiệu lực", "Tạm ngưng hiệu lực", "Tạm hoãn thi hành"},
	} {
		for _, verb := range verbs {
			variants := []string{verb}
			if kind == "suspends" {
				variants = append(variants, verb+" thi hành", verb+" áp dụng")
			}
			for _, phrase := range variants {
				t.Run(phrase, func(t *testing.T) {
					matches, gaps := Parse(phrase+" Điều 3 của Thông tư 39/2016/TT-NHNN.", testIssuers())
					if len(matches) != 1 || len(gaps) != 0 || matches[0].Kind != kind || matches[0].VerbPhrase != strings.ToLower(phrase) {
						t.Fatalf("matches = %#v, gaps = %#v", matches, gaps)
					}
				})
			}
		}
	}
}

func TestParse_QuotedCandidatesStayExcluded(t *testing.T) {
	for _, quoted := range []string{
		"\"Nội dung. Bãi bỏ Điều 3 của Thông tư 39/2016/TT-NHNN.\"",
		"“Nội dung. Bãi bỏ Điều 3 của Thông tư 39/2016/TT-NHNN.”",
		"«Nội dung. Bãi bỏ Điều 3 của Thông tư 39/2016/TT-NHNN.»",
		"'Nội dung. Bãi bỏ Điều 3 của Thông tư 39/2016/TT-NHNN.'",
	} {
		matches, gaps := Parse(quoted, testIssuers())
		if len(matches) != 0 || len(gaps) != 0 {
			t.Fatalf("%q: matches = %#v, gaps = %#v", quoted, matches, gaps)
		}
	}
}

func TestDate_SharedSourceFormats(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"1/2/2023", "2023-02-01"},
		{"01 tháng 02 năm 2023", "2023-02-01"},
		{"2023-02-01", "2023-02-01"},
		{"2023-02-01T17:00:00", "2023-02-01"},
		{"2023-02-01T23:30:00Z", "2023-02-02"},
		{"2023-02-01T23:30:00+07:00", "2023-02-01"},
		{"2023-02-29", ""},
		{"0000-01-01", ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			c := testCursor(tc.text)
			date, ok := c.date(false)
			if date != tc.want || ok != (tc.want != "") {
				t.Fatalf("date = %q, %v; want %q", date, ok, tc.want)
			}
		})
	}
}

func TestParse_LongMalformedTail(t *testing.T) {
	const fragments = 8000
	text := "Bãi bỏ Điều 3 của Thông tư 39/2016/TT-NHNN về " + strings.Repeat("1/", fragments) + "1."
	start := time.Now()
	matches, gaps := Parse(text, testIssuers())
	elapsed := time.Since(start)
	if len(matches) != 1 || len(gaps) != 0 {
		t.Fatalf("got %d matches and %d gaps; want one match and no gaps", len(matches), len(gaps))
	}
	if matches[0].Targets[0].Number != "39/2016/TT-NHNN" || matches[0].Sentence != text {
		t.Fatal("malformed tail changed the target or evidence")
	}
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("parsing %d malformed fragments took %s; want under 100 ms", fragments, elapsed)
	}
	t.Logf("parsed %d malformed fragments in %s", fragments, elapsed)
}

func TestTail_DoesNotRetryMalformedNumberSuffix(t *testing.T) {
	c := testCursor("1/1/39/2016/TT-NHNN")
	date, gap := c.tail()
	if date != nil || gap != "" {
		t.Fatalf("date = %v, gap = %q; malformed token must not yield a suffix number", date, gap)
	}
}
