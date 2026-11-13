package corpusdb

import (
	"slices"
	"strings"
	"testing"
)

func TestSchema_PinsKeepVersion5Supported(t *testing.T) {
	if SchemaVersion != 6 || !slices.Contains(SupportedSchemaVersions(), 5) || !slices.Contains(SupportedSchemaVersions(), 6) {
		t.Fatalf("schema=%d supported=%v", SchemaVersion, SupportedSchemaVersions())
	}
	for _, part := range []string{"schema_version = 6", "source_withdrawn_since date", "source_withdrawn_rel text"} {
		if !strings.Contains(schema, part) {
			t.Errorf("schema missing %s", part)
		}
	}
	hash, err := SchemaSHA256(1024)
	if err != nil || hash != "066a2887494acb127f830f7a75fcb1dabe1b3e0f50b0f136fbcf375f636b61ed" {
		t.Fatalf("schema SHA256=%s err=%v", hash, err)
	}

}
