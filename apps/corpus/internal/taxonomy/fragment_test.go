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

func TestIsFragment_OnlyWholeBoilerplate(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       bool
	}{
		{"effect predicate before", "Thông tư này quy định mức phí là 100 đồng và có hiệu lực kể từ ngày 1 tháng 1 năm 2027.", false},
		{"effect predicate after", "Thông tư này có hiệu lực kể từ ngày 1 tháng 1 năm 2027 và quy định mức phí.", false},
		{"effect named part", "Khoản 2 Điều 3 có hiệu lực từ ngày 1 tháng 1 năm 2027.", true},
		{"responsibility predicate before", "Ngân hàng phải công bố báo cáo hằng tháng và chịu trách nhiệm thi hành Thông tư này.", false},
		{"responsibility second predicate", "Bộ trưởng và ngân hàng thu phí chịu trách nhiệm thi hành Thông tư này.", false},
		{"responsibility allowed words form predicate", "Bộ trưởng và ngân hàng có tổ chức chịu trách nhiệm thi hành Thông tư này.", false},
		{"responsibility after", "Bộ trưởng chịu trách nhiệm thi hành Thông tư này và công bố báo cáo.", false},
		{"responsibility digits", "Bộ trưởng 2 chịu trách nhiệm thi hành Thông tư này.", false},
		{"responsibility guide", "Bộ trưởng chịu trách nhiệm hướng dẫn thi hành Thông tư này.", true},
		{"responsibility implement", "Bộ trưởng chịu trách nhiệm triển khai thực hiện Thông tư này.", true},
		{"repeal before", "Ngân hàng thu phí và bãi bỏ Điều 2 của Thông tư này.", false},
		{"repeal after", "Bãi bỏ Điều 2 của Thông tư này và thu phí hằng tháng.", false},
		{"repeal second predicate", "Bãi bỏ Điều 2 và ban hành Thông tư này.", false},
		{"replacement second predicate", "Thay thế Phụ lục 1 bằng Phụ lục 2 và ban hành Thông tư này.", false},
		{"repeal reference", "Bãi bỏ khoản 2 Điều 3 của Thông tư số 50/2024/TT-NHNN.", true},
		{"replacement before", "Ngân hàng thu phí và thay thế Phụ lục 1 bằng Phụ lục 2.", false},
		{"replacement after", "Thay thế Phụ lục 1 bằng Phụ lục 2 và thu phí hằng tháng.", false},
		{"replacement reference", "Thay thế Phụ lục 1 bằng Phụ lục 2.", true},
		{"amendment before", "Ngân hàng thu phí và Điều 2 được sửa đổi, bổ sung như sau:", false},
		{"amendment after same line", "Điều 2 được sửa đổi, bổ sung như sau: Người dân nộp phí.", false},
		{"amendment followed by boilerplate", "Điều 2 được sửa đổi, bổ sung như sau:\nThông tư này có hiệu lực từ ngày 1 tháng 1 năm 2027.", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFragment("Điều 1. Heading\n" + tt.body); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
