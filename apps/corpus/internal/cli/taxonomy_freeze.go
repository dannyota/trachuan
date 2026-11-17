package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func runTaxonomyFreeze(args []string, stderr io.Writer) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	opts, err := options(args, map[string]bool{"--packet": true, "--labels": true, "--labeler": true, "--out": true})
	if err != nil {
		return fail(2, err)
	}
	if opts["--packet"] == "" || opts["--labels"] == "" || opts["--labeler"] == "" || opts["--out"] == "" {
		return fail(2, fmt.Errorf("--packet, --labels, --labeler and --out are required"))
	}
	var packet taxonomy.Packet
	if err = readTaxonomyJSON(opts["--packet"], &packet); err != nil {
		return fail(1, err)
	}
	if filepath.Base(opts["--out"]) != "taxonomy-"+packet.Slice+".json" {
		return fail(2, fmt.Errorf("output must be named taxonomy-%s.json", packet.Slice))
	}
	var labels struct {
		Articles []taxonomy.LabelArticle `json:"articles"`
	}
	if err = readTaxonomyJSON(opts["--labels"], &labels); err != nil {
		return fail(1, err)
	}
	frozen, err := taxonomy.FreezePacket(packet, labels.Articles, opts["--labeler"])
	if err != nil {
		return fail(1, err)
	}
	if err = writeTaxonomyJSON(opts["--out"], frozen); err != nil {
		return fail(1, err)
	}
	return 0
}
func readTaxonomyJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	if err = json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode input: %w", err)
	}
	return nil
}
