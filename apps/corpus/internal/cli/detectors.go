package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

func runDetectors(_ context.Context, args []string, stdout, stderr io.Writer) int {
	usage := func() int {
		_, _ = fmt.Fprintln(stderr, "usage: corpus detectors list [--json]")
		return 2
	}
	if len(args) == 0 || args[0] != "list" {
		return usage()
	}
	opts, err := options(args[1:], map[string]bool{"--json": false})
	if err != nil {
		return usage()
	}
	if opts["--json"] != "" {
		data, marshalErr := json.Marshal(struct {
			Hash     string          `json:"hash"`
			Registry json.RawMessage `json:"registry"`
		}{detector.Hash(), detector.Canonical()})
		if marshalErr != nil {
			_, _ = fmt.Fprintln(stderr, marshalErr)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", data)
		return 0
	}
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "ID\tVERSION\tSEMANTICS\tSEVERITY\tSUBJECT")
	for _, d := range detector.All() {
		_, _ = fmt.Fprintf(table, "%s\t%d\t%d\t%s\t%s\n", d.ID, d.Version, d.Semantics, d.Severity, d.Subject)
	}
	if err = table.Flush(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "registry hash %s\n", detector.Hash())
	return 0
}
