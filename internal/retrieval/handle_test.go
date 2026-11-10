package retrieval

import "testing"

func TestHandle_RoundTripAnchors(t *testing.T) {
	const key = "vn:vbpl:03206030-6f7b-11f1-95c5-bd4e620e4897"
	for _, anchor := range []string{"", "dieu-1/khoan-1/diem-a", "dieu-3~2", "phụ-lục-1"} {
		t.Run(anchor, func(t *testing.T) {
			want := key
			if anchor != "" {
				want += "#" + anchor
			}
			got := Handle(Identity{DocumentKey: key, Anchor: anchor})
			if got != want {
				t.Fatalf("Handle() = %q, want %q", got, want)
			}
			gotKey, gotAnchor, err := ParseHandle(got)
			if err != nil || gotKey != key || gotAnchor != anchor {
				t.Fatalf("ParseHandle() = %q, %q, %v", gotKey, gotAnchor, err)
			}
		})
	}
}

func TestParseHandle_Rejects(t *testing.T) {
	for _, input := range []string{"", "other:key", "#dieu-1", "vn:key#"} {
		t.Run(input, func(t *testing.T) {
			if _, _, err := ParseHandle(input); err == nil {
				t.Fatal("ParseHandle() accepted invalid handle")
			}
		})
	}
}
