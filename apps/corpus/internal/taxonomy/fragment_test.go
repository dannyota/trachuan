package taxonomy

import (
	"encoding/json"
	"os"
	"testing"
)

func TestIsFragment_Articles(t *testing.T) {
	data, err := os.ReadFile("testdata/fragments.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Articles []struct {
			Text     string `json:"text"`
			Fragment bool   `json:"fragment"`
			Anchor   string `json:"anchor"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures.Articles) != 11 {
		t.Fatalf("got %d fixtures", len(fixtures.Articles))
	}
	for _, a := range fixtures.Articles {
		if got := IsFragment(a.Text); got != a.Fragment {
			t.Errorf("%s: got %v", a.Anchor, got)
		}
	}
	for _, tt := range []struct {
		text string
		want bool
	}{
		{"Điều 1. Heading\nVăn bản này có hiệu lực kể từ ngày 1 tháng 1 năm 2027.", true},
		{"Điều 1. Heading\n1. Thông tư này có hiệu lực kể từ ngày 1 tháng 1 năm 2027. 2. Bộ trưởng chịu trách nhiệm thi hành Thông tư này.", true},
		{"Điều 1. Heading\nThông tư này hết hiệu lực từ ngày 1 tháng 1 năm 2027.", true},
		{"Điều 1. Heading\nBộ trưởng hướng dẫn thi hành Nghị định này.", true},
		{"Điều 1. Heading\nBãi bỏ Điều 2 của Thông tư này.", true},
		{"Điều 1. Heading\nĐiều 2 được sửa đổi, bổ sung như sau:", true},
		{"Điều 1. Heading\nĐiều 2 được sửa đổi, bổ sung như sau:\nNgười dân được cấp phép.", false},
		{"Điều 1. Heading\nCác hợp đồng ký trước ngày Thông tư này có hiệu lực thi hành được tiếp tục thực hiện.", false},
		{"Điều 1. Heading\nCơ quan chịu trách nhiệm thi hành Thông tư này và phải công bố báo cáo.", false},
		{"Điều 1. Heading\nCác hợp đồng có hiệu lực kể từ ngày 1 tháng 1 năm 2027 được tiếp tục thực hiện.", false},
	} {
		if got := IsFragment(tt.text); got != tt.want {
			t.Errorf("%q: got %v", tt.text, got)
		}
	}
}
