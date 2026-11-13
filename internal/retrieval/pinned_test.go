package retrieval

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDocumentEntry_SourceWithdrawal(t *testing.T) {
	for _, schema := range []int{5, 6} {
		for _, pinned := range []bool{false, true} {
			var since *time.Time
			var rel *string
			if pinned {
				date := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)
				value := "20990101_1"
				since = &date
				rel = &value
			}
			doc := DocumentEntry{SourceWithdrawn: sourceWithdrawal(schema, since, rel)}
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), `"source_withdrawn"`) != (schema >= 6 && pinned) {
				t.Fatalf("schema=%d pinned=%v document=%s", schema, pinned, data)
			}
			if schema >= 6 && pinned && !strings.Contains(string(data), `"acquired_by_rel":"20990101_1"`) {
				t.Fatalf("document=%s", data)
			}
		}
	}
}

func TestDocumentMetadataSQL_Version5OmitsPinColumns(t *testing.T) {
	for _, version := range []int{4, 5, 6} {
		fields := withdrawnColumns(version)
		if strings.Contains(fields, "source_withdrawn_since") != (version >= 6) || strings.Contains(fields, "source_withdrawn_rel") != (version >= 6) {
			t.Fatalf("schema %d columns=%s", version, fields)
		}
	}
}
