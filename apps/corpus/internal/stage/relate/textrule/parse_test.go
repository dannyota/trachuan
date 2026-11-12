package textrule

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func testIssuers() map[string]string {
	return map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", "CP": "CHÍNH PHỦ", "TTg": "THỦ TƯỚNG CHÍNH PHỦ", "BTP": "BỘ TƯ PHÁP", "QH": "QUỐC HỘI", "UBTVQH": "ỦY BAN THƯỜNG VỤ QUỐC HỘI"}
}

func readFixtures[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []T
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func TestParse_AcceptFixtures(t *testing.T) {
	for _, fixture := range readFixtures[struct {
		Name, Text string
		Want       []Match
	}](t, "testdata/accept.json") {
		t.Run(fixture.Name, func(t *testing.T) {
			matches, gaps := Parse(fixture.Text, testIssuers())
			if len(gaps) != 0 || !reflect.DeepEqual(matches, fixture.Want) {
				t.Fatalf("matches = %#v, gaps = %#v; want %#v", matches, gaps, fixture.Want)
			}
		})
	}
}

func TestParse_RejectFixtures(t *testing.T) {
	for _, fixture := range readFixtures[struct {
		Name, Text, Want string
	}](t, "testdata/reject.json") {
		t.Run(fixture.Name, func(t *testing.T) {
			matches, gaps := Parse(fixture.Text, testIssuers())
			if len(matches) != 0 {
				t.Fatalf("unexpected matches: %#v", matches)
			}
			if fixture.Want == "" {
				if len(gaps) != 0 {
					t.Fatalf("silently skipped sentence produced gaps: %#v", gaps)
				}
				return
			}
			if len(gaps) != 1 || gaps[0].Name != fixture.Want || gaps[0].Detail == "" {
				t.Fatalf("gaps = %#v, want one %s gap with detail", gaps, fixture.Want)
			}
		})
	}
}

func TestParse_SentencesAreIndependent(t *testing.T) {
	text := "Không thay đổi. Bãi bỏ Điều 3 của Thông tư 39/2016/TT-NHNN. Sửa đổi khoản 2. Thay thế Điều 4 của Thông tư 39/2016/TT-NHNN."
	matches, gaps := Parse(text, testIssuers())
	if len(matches) != 2 || len(gaps) != 1 || gaps[0].Name != "relation_text_grammar" {
		t.Fatalf("matches = %#v, gaps = %#v", matches, gaps)
	}
	if matches[0].Targets[0].Anchor != "dieu-3" || matches[1].Targets[0].Anchor != "dieu-4" {
		t.Fatalf("wrong targets: %#v", matches)
	}
}

func TestParse_IssuerTableIsClosed(t *testing.T) {
	text := "Bãi bỏ Điều 3 của Thông tư 39/2016/TT-NHNN."
	for _, issuers := range []map[string]string{nil, {}, {"CP": "CHÍNH PHỦ"}} {
		matches, gaps := Parse(text, issuers)
		if len(matches) != 0 || len(gaps) != 1 || gaps[0].Name != "relation_text_local_number" {
			t.Fatalf("matches = %#v, gaps = %#v", matches, gaps)
		}
	}
}
