package cli

import (
	"context"
	"fmt"
	"io"

	"danny.vn/trachuan/internal/frameworks/build"
)

func runFrameworks(ctx context.Context, args []string, out, errout io.Writer) int {
	if len(args) == 2 && args[0] == "release-row" {
		return runFrameworksReleaseRow(ctx, args[1], errout)
	}
	if len(args) != 2 || args[0] != "build" {
		_, _ = fmt.Fprintln(errout, "usage: corpus frameworks build <rel> | corpus frameworks release-row <rel>")
		return 2
	}
	if err := build.Run(ctx, args[1], out, newFrameworkEmbedder(), newFrameworkDatabase(), openFrameworkCalibration); err != nil {
		_, _ = fmt.Fprintln(errout, err)
		return 1
	}
	return 0
}
