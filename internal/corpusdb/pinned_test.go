package corpusdb

import (
	"slices"
	"strings"
	"testing"
)

func TestSchema_PinsKeepVersion5Supported(t *testing.T) {
	if SchemaVersion != 7 || !slices.Equal(SupportedSchemaVersions(), []int{4, 5, 6, 7}) {
		t.Fatalf("schema=%d supported=%v", SchemaVersion, SupportedSchemaVersions())
	}
	for _, part := range []string{"schema_version = 7", "source_withdrawn_since date", "source_withdrawn_rel text"} {
		if !strings.Contains(schema, part) {
			t.Errorf("schema missing %s", part)
		}
	}
	hash, err := SchemaSHA256(1024)
	if err != nil || hash != "550515aeeef8ed9232c334c7a88229f76c7074cb471de9b71ecd8a67cbf7a40d" {
		t.Fatalf("schema SHA256=%s err=%v", hash, err)
	}

}
