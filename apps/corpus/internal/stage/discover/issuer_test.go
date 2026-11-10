package discover

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
)

func TestIssuer_RepeatedConflictingFieldsUnknown(t *testing.T) {
	fields := []chinhphu.FieldOccurrence{{Name: "Cơ quan ban hành", Value: "Chính phủ"}, {Name: "Cơ quan ban hành", Value: "Quốc hội"}}
	if got := classifyIssuer(fields, []string{"Chính phủ", "Quốc hội"}); got != "unknown" {
		t.Fatalf("issuer = %q", got)
	}
}

func TestIssuer_WholeFieldAndProvincial(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"  chính   phủ ", "central"},
		{"Chính phủ và Quốc hội", "unknown"},
		{"Chủ tịch UBND tỉnh Phú Thọ", "provincial"},
		{"UBND tỉnh", "unknown"},
		{"", "unknown"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got := classifyIssuer([]chinhphu.FieldOccurrence{{Name: "Cơ quan ban hành", Value: tc.value}}, []string{"Chính phủ"})
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
