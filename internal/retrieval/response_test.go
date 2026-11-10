package retrieval

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDate_JSONIsCivilDate(t *testing.T) {
	d := Date{Year: 2026, Month: time.November, Day: 9}
	b, err := json.Marshal(d)
	if err != nil || string(b) != `"2026-11-09"` {
		t.Fatalf("marshal = %s, %v", b, err)
	}
	var back Date
	if err := json.Unmarshal(b, &back); err != nil || back != d {
		t.Fatalf("unmarshal = %+v, %v", back, err)
	}
	if err := json.Unmarshal([]byte(`"2026-02-30"`), &back); err == nil {
		t.Fatal("accepted an invalid calendar date")
	}
}

func TestIdentity_JSONFieldNames(t *testing.T) {
	b, err := json.Marshal(Identity{Country: "vn", DocumentKey: "vn:vbpl:1", Anchor: "dieu-1"})
	if err != nil || string(b) != `{"country":"vn","document_key":"vn:vbpl:1","anchor":"dieu-1"}` {
		t.Fatalf("marshal = %s, %v", b, err)
	}
}
