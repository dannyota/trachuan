package frameworks

import (
	"context"
	"strings"
	"testing"
)

func TestExecutableSQL_SeparatesTrustedObjects(t *testing.T) {
	sql, err := ExecutableSQL(SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range []string{"VIEW body_lexical_stats", "FUNCTION check_evidence_dependencies()", "TRIGGER release_versions_sealed"} {
		if !strings.Contains(sql, object) {
			t.Fatalf("missing %s", object)
		}
	}
	for _, forbidden := range []string{"CREATE TABLE", "CREATE INDEX", "ALTER TABLE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("executable SQL contains %s", forbidden)
		}
	}
	for _, version := range []int{-1, 0, 2} {
		if _, err := ExecutableSQL(version); err == nil {
			t.Fatalf("accepted version %d", version)
		}
	}
	db := &recordingDB{}
	if err := Apply(context.Background(), db, 1024); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(db.statements, ""), sql) {
		t.Fatal("build does not apply the executable SQL")
	}
}
