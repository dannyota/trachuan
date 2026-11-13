package build

import (
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestPinned_ReleaseMetadataAndGap(t *testing.T) {
	pin := source.Pin{Key: "vn:vbpl:pin", Number: "71/2099/QH15", Since: "2099-01-02", Rel: "20990101_1", Expand: "expand", Fetch: "fetch"}
	l := loader{pins: map[string]source.Pin{pin.Key: pin}, present: map[string]bool{pin.Key: true}}
	since, rel, err := l.pinnedMetadata(pin.Key, "fetch")
	if err != nil || since == nil || since.Format("2006-01-02") != pin.Since || rel == nil || *rel != pin.Rel {
		t.Fatalf("metadata=%v %v %v", since, rel, err)
	}
	if len(l.gaps) != 1 || l.gaps[0].Kind != "source_withdrawn_retained" {
		t.Fatalf("gaps=%+v", l.gaps)
	}
	for _, value := range []string{pin.Since, pin.Rel, pin.Expand, pin.Fetch} {
		if !strings.Contains(l.gaps[0].Detail, value) {
			t.Errorf("gap lacks %s", value)
		}
	}
	if _, _, err = l.pinnedMetadata(pin.Key, "wrong"); err == nil {
		t.Fatal("accepted a different fetch artifact")
	}
	since, rel, err = l.pinnedMetadata("vn:vbpl:ordinary", "fetch")
	if err != nil || since != nil || rel != nil {
		t.Fatalf("ordinary metadata=%v %v %v", since, rel, err)
	}
}
