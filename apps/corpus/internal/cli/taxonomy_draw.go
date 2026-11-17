package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type drawEvidence struct {
	articles     []taxonomy.DrawArticle
	tree         []taxonomy.PacketNode
	nodes        []classify.Node
	taxonomyHash string
	manualCount  int64
}

func runTaxonomyDraw(ctx context.Context, args []string, stderr io.Writer) int {
	return runTaxonomyDrawWith(ctx, args, stderr, loadDrawEvidence)
}
func runTaxonomyDrawWith(ctx context.Context, args []string, stderr io.Writer, load func(context.Context, string, string) (drawEvidence, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) < 2 {
		return fail(2, fmt.Errorf("usage: corpus taxonomy draw <cc> <rel> --slice SLICE --seed N --out FILE [--families FILE] [--exclude FILE]"))
	}
	if _, err := build.DatabaseName(args[0], args[1]); err != nil {
		return fail(2, err)
	}
	opts, err := options(args[2:], map[string]bool{"--slice": true, "--seed": true, "--out": true, "--families": true, "--exclude": true})
	if err != nil {
		return fail(2, err)
	}
	if opts["--out"] == "" || opts["--seed"] == "" || (opts["--slice"] != "multi-family" && opts["--slice"] != "post-2023" && opts["--slice"] != "inheritance") {
		return fail(2, fmt.Errorf("--slice, --seed and --out are required"))
	}
	if (opts["--slice"] == "multi-family") != (opts["--families"] != "") {
		return fail(2, fmt.Errorf("--families is required only for multi-family"))
	}
	seed, err := strconv.ParseInt(opts["--seed"], 10, 64)
	if err != nil {
		return fail(2, fmt.Errorf("invalid seed: %w", err))
	}
	e, err := load(ctx, args[0], args[1])
	if err != nil {
		return fail(1, err)
	}
	if e.manualCount != 0 {
		return fail(1, fmt.Errorf("release has %d manual taxonomy assignments; seed provenance is incomplete", e.manualCount))
	}
	if opts["--slice"] == "multi-family" {
		f, _, readErr := classify.ReadFamilyArtifactForScore(opts["--families"], e.taxonomyHash, e.nodes)
		if readErr != nil {
			return fail(1, readErr)
		}
		for i := range e.articles {
			e.articles[i].MultiFamily = len(f.Documents[e.articles[i].Key].Codes) >= 2
		}
	}
	var exclusions struct {
		Articles []taxonomy.ArticleID `json:"articles"`
	}
	if path := opts["--exclude"]; path != "" {
		if readErr := readTaxonomyJSON(path, &exclusions); readErr != nil {
			return fail(1, readErr)
		}
		if exclusions.Articles == nil {
			return fail(1, fmt.Errorf("exclusion file must contain an articles array"))
		}
	}
	articles, skipped, labelerFragments, err := taxonomy.DrawAdjudicatedExcluding(e.articles, opts["--slice"], seed, exclusions.Articles)
	if err != nil {
		return fail(1, err)
	}
	packet := taxonomy.Packet{LabelerFragments: labelerFragments, LabelingRule: taxonomy.LabelingRule, FragmentsSkipped: skipped, Country: args[0], Release: args[1], Slice: opts["--slice"], Seed: seed, DrawRule: taxonomy.AdjudicatedDrawRule, Articles: articles, Tree: e.tree}
	if err = writeTaxonomyJSON(opts["--out"], packet); err != nil {
		return fail(1, err)
	}
	return 0
}
func writeTaxonomyJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode output: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return fmt.Errorf("write output: %w", writeErr)
		}
		return fmt.Errorf("close output: %w", closeErr)
	}
	return nil
}
