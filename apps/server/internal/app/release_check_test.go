package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestReleaseCheck_RejectsArgumentsBeforeLoadingServeConfig(t *testing.T) {
	for _, args := range [][]string{{"--database"}, {"--database", "auth"}, {"extra"}} {
		var output bytes.Buffer
		err := run(context.Background(), append([]string{"release-check"}, args...), &output, &output)
		if err == nil || !strings.Contains(err.Error(), "release-check arguments") {
			t.Fatalf("args %v: %v", args, err)
		}
	}
}
