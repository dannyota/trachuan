package build

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSummaries_SharedPath(t *testing.T) {
	d := summaryFixture()
	d.Payload.Outline = []summaryOutline{{Kind: "dieu", Label: "Điều 1", Anchor: "dieu-1", Heading: "Điều khoản tóm tắt công khai E2E"}, {Kind: "dieu", Label: "Điều 2", Anchor: "dieu-2", Heading: strings.Repeat("x", 320)}}
	d.Payload.Nodes = []summaryNode{{Code: "99.1", Name: "Chủ đề tóm tắt E2E"}}
	a := &summaryTx{documents: []summaryDocument{d}, sizes: []int{65536}}
	b := &summaryTx{documents: []summaryDocument{d}, sizes: []int{65536}}
	stats, gaps, err := LoadSummaries(context.Background(), a, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := loader{tx: b, docs: map[string]*loadedDocument{d.Key: {id: 1}}}
	if err = l.loadSummaries(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.inserts, b.inserts) || stats != l.counts.Summary || !reflect.DeepEqual(gaps, l.gaps) {
		t.Fatal("exported and build paths differ")
	}
	var payload summaryPayload
	if err = json.Unmarshal(a.inserts[0][11].([]byte), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Outline) != 2 || payload.Outline[0].Heading != "Điều khoản tóm tắt công khai E2E" || payload.Outline[1].Heading != "" || len(payload.Nodes) != 1 || payload.Nodes[0].Name != "Chủ đề tóm tắt E2E" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if strings.Contains(string(a.inserts[0][11].([]byte)), "E2E-BODY-MARKER-7Q4") {
		t.Fatal("body leaked")
	}
}
