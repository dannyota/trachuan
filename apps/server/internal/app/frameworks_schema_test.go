package app

import (
	"bytes"
	"context"
	"testing"

	"danny.vn/trachuan/internal/corpusdb/frameworks"
)

func TestFrameworksSchema_PrintsOnlyVersionedSQL(t *testing.T) {
	t.Setenv("SERVER_DATABASE_URL", "")
	var out bytes.Buffer
	if err := run(context.Background(), []string{"frameworks-schema", "--version", "1"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	want, err := frameworks.ExecutableSQL(1)
	if err != nil || out.String() != want {
		t.Fatal("output differs from executable SQL")
	}
	for _, args := range [][]string{{}, {"--version", "2"}, {"--version", "x"}, {"--version", "1", "extra"}} {
		out.Reset()
		if err := run(context.Background(), append([]string{"frameworks-schema"}, args...), &out, &out); err == nil || out.Len() != 0 {
			t.Fatalf("accepted args %v", args)
		}
	}
}
