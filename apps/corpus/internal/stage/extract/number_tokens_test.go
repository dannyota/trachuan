package extract

import (
	"reflect"
	"strings"
	"testing"
)

func TestHeaderNumbers_TokenBoundaries(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   []string
	}{
		{"số 47 / 2025/TT-BKHCN ngày 1", nil},
		{"QUYẾT ĐỊNH Về việc đính chính Thông tư số 14/2015/TT-NHNN", nil},
		{"Số: 10-NH5 THÔNG TƯ SỐ 10-NH5 đã ký quyết định số 107/QĐ-NH5", []string{"10-NH5", "10-NH5"}},
		{"Nghị định số 196-HĐBT", nil},
		{"SỐ: 47/2025/TT-BKHCN", []string{"47/2025/TT-BKHCN"}},
		{"Số 47/2025/TT-BKHCN", nil},
		{"SỐ 47 / 2025/TT-BKHCN ngày 1", []string{"47 / 2025/TT-BKHCN"}},
		{"SỐ: 14TC / TTTN nội dung", []string{"14TC / TTTN"}},
		{"Số: 1 tiêu đề", nil},
		{"hồsố: 47/2025/TT-BKHCN", nil},
	} {
		t.Run(tc.header, func(t *testing.T) {
			if got := headerNumbers(tc.header); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTitleHasNumber_WholeToken(t *testing.T) {
	for _, tc := range []struct {
		title string
		want  bool
	}{
		{"Theo 47 / 2025 / TT - BKHCN", true},
		{"Theo 147/2025/TT-BKHCN", false},
		{"Theo a47/2025/TT-BKHCN", false},
		{"Theo 47/2025/TT-BKHCNX", false},
		{"Theo (47/2025/TT-BKHCN).", true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			if got := titleHasNumber(tc.title, "47/2025/TT-BKHCN"); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestHeaderNumbers_Bounds(t *testing.T) {
	header := strings.Repeat("SỐ 47/SL ", 10000)
	if got := headerNumbers(header); len(got) != 50 {
		t.Fatalf("got %d candidates, want 50", len(got))
	}
	if got := headerNumbers(strings.Repeat("ả", 4000) + " SỐ 47/SL"); len(got) != 0 {
		t.Fatalf("number beyond header limit: %q", got)
	}
	if got := headerNumbers(strings.Repeat("SỐ 1 ", 50) + "SỐ 47/SL"); len(got) != 0 {
		t.Fatalf("number beyond marker limit: %q", got)
	}
}
