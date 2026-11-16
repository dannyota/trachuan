package corpusdb

import (
	"slices"
	"strings"
	"testing"
)

func TestSchema_PinsKeepVersion5Supported(t *testing.T) {
	if SchemaVersion != 8 || !slices.Equal(SupportedSchemaVersions(), []int{4, 5, 6, 7, 8}) {
		t.Fatalf("schema=%d supported=%v", SchemaVersion, SupportedSchemaVersions())
	}
	for _, part := range []string{"schema_version = 8", "source_withdrawn_since date", "source_withdrawn_rel text"} {
		if !strings.Contains(schema, part) {
			t.Errorf("schema missing %s", part)
		}
	}
	hash, err := SchemaSHA256(1024)
	if err != nil || hash != "46fd4c494fb4774456d05dadad3c3da03c5df4deadaee59674b4e22725ade66c" {
		t.Fatalf("schema SHA256=%s err=%v", hash, err)
	}

}
