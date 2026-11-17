package taxonomy

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestAdjudicatedDraw_Slices(t *testing.T) {
	var rows []DrawArticle
	for i := 0; i < 120; i++ {
		for j := 0; j < 3; j++ {
			rows = append(rows, DrawArticle{PacketArticle: PacketArticle{Key: fmt.Sprintf("doc-%03d", i), Anchor: fmt.Sprintf("dieu-%d", j), Text: "own"}, Primary: true, IssueDate: "2024-01-01", MultiFamily: i < 40, Inherited: i < 60})
		}
	}
	for _, slice := range []string{"multi-family", "post-2023", "inheritance"} {
		t.Run(slice, func(t *testing.T) {
			got, err := DrawAdjudicated(rows, slice, 7)
			if err != nil {
				t.Fatal(err)
			}
			again, err := DrawAdjudicated(rows, slice, 7)
			if err != nil || !reflect.DeepEqual(got, again) {
				t.Fatal("draw is not deterministic")
			}
			counts := map[string]int{}
			multifamily := 0
			for _, a := range got {
				if slice == "inheritance" {
					var n int
					_, _ = fmt.Sscanf(a.Key, "doc-%d", &n)
					if n >= 60 {
						t.Fatalf("ineligible inheritance: %s", a.Key)
					}
				}
				counts[a.Key]++
				if counts[a.Key] > 2 {
					t.Fatal("document cap exceeded")
				}
				if strings.HasPrefix(a.Key, "doc-0") {
					var n int
					_, _ = fmt.Sscanf(a.Key, "doc-%d", &n)
					if n < 40 {
						multifamily++
					}
				}
			}
			if len(got) != 100 {
				t.Fatalf("got %d", len(got))
			}
			if slice == "multi-family" && multifamily < 50 {
				t.Fatalf("multi-family %d", multifamily)
			}
		})
	}
	rows[0].IssueDate = "2023-12-31"
	rows[1].Primary = false
	rows[2].Seeded = true
	got, err := DrawAdjudicated(rows, "post-2023", 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if a.Key == rows[0].Key {
			t.Fatal("ineligible article selected")
		}
	}
	for i := range rows {
		rows[i].Inherited = false
	}
	if _, err = DrawAdjudicated(rows, "inheritance", 7); err == nil || !strings.Contains(err.Error(), "0") {
		t.Fatalf("missing count: %v", err)
	}
}

func TestFreeze_ValidationAndBlindPacket(t *testing.T) {
	packet := Packet{Country: "vn", Release: "20990101", Slice: "post-2023", DrawRule: "rule", Articles: []PacketArticle{{Key: "doc", Anchor: "dieu-1", Text: "own", OwnTextSHA256: OwnTextSHA256("own")}}, Tree: []PacketNode{{Code: "1.1", Level: 2, Name: "family"}}}
	data, _ := json.Marshal(packet)
	for _, s := range []string{"seed_codes", "inherited", "classifier"} {
		if strings.Contains(string(data), s) {
			t.Fatal(string(data))
		}
	}
	for _, tt := range []struct {
		name   string
		labels []LabelArticle
		valid  bool
	}{
		{"valid", []LabelArticle{{Key: "doc", Anchor: "dieu-1", Codes: []string{"1.1"}}}, true},
		{"missing", nil, false}, {"extra", []LabelArticle{{Key: "other", Anchor: "dieu-1", Codes: []string{"1.1"}}}, false},
		{"unknown", []LabelArticle{{Key: "doc", Anchor: "dieu-1", Codes: []string{"bad"}}}, false},
		{"empty", []LabelArticle{{Key: "doc", Anchor: "dieu-1"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FreezePacket(packet, tt.labels, "gpt-6-astra")
			if (err == nil) != tt.valid {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

func TestAdjudicatedDraw_RandomHalfIncludesRemainingMultiFamily(t *testing.T) {
	var rows []DrawArticle
	for i := 0; i < 200; i++ {
		rows = append(rows, DrawArticle{PacketArticle: PacketArticle{Key: fmt.Sprintf("doc-%03d", i), Anchor: "dieu-1", Text: "operative text"}, Primary: true, MultiFamily: i < 100})
	}
	got, err := DrawAdjudicated(rows, "multi-family", 7)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, a := range got {
		var n int
		_, _ = fmt.Sscanf(a.Key, "doc-%d", &n)
		if n < 100 {
			count++
		}
	}
	if count < 60 {
		t.Fatalf("random half under-samples remaining multi-family articles: %d", count)
	}
}

func TestFreeze_NoNode(t *testing.T) {
	packet := Packet{Country: "vn", Release: "20990101", Slice: "post-2023", DrawRule: "rule", Articles: []PacketArticle{{Key: "doc", Anchor: "dieu-1", Text: "own", OwnTextSHA256: OwnTextSHA256("own")}}, Tree: []PacketNode{{Code: "1.1", Level: 2}, {Code: "1.1.c1", Level: 3}}}
	for _, tt := range []struct {
		codes []string
		valid bool
	}{{[]string{"1.1"}, true}, {[]string{"1.1.c1"}, false}, {[]string{"1.1", "1.1.c1"}, false}} {
		raw, _ := json.Marshal(map[string]any{"document_key": "doc", "anchor": "dieu-1", "codes": tt.codes, "no_node": true})
		var label LabelArticle
		if err := json.Unmarshal(raw, &label); err != nil {
			t.Fatal(err)
		}
		got, err := FreezePacket(packet, []LabelArticle{label}, "gpt-6-astra")
		if (err == nil) != tt.valid {
			t.Errorf("%v: %v", tt.codes, err)
			continue
		}
		if tt.valid {
			b, _ := json.Marshal(got)
			if !strings.Contains(string(b), `"no_node":true`) {
				t.Errorf("missing no_node: %s", b)
			}
		}
	}
}
