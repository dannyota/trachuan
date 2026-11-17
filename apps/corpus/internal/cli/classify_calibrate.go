package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func runClassifyCalibrate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runClassifyCalibrateWith(ctx, args, stdout, stderr, loadClassificationEvidence)
}

func runClassifyCalibrateWith(ctx context.Context, args []string, stdout, stderr io.Writer, load func(context.Context, string, string, taxonomy.Labels) (classificationEvidence, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) < 2 {
		return fail(2, fmt.Errorf("usage: corpus classify calibrate <cc> <rel> --seeded LABELS:FAMILIES:ARTICLES --adjudicated LABELS:FAMILIES:ARTICLES [--adjudicated LABELS:FAMILIES:ARTICLES ...] --out DIR"))
	}
	if _, err := build.DatabaseName(args[0], args[1]); err != nil {
		return fail(2, err)
	}
	var seeded, out string
	var adjudicated []string
	for i := 2; i < len(args); i += 2 {
		if i+1 >= len(args) || args[i+1] == "" {
			return fail(2, fmt.Errorf("calibrate requires option values"))
		}
		switch args[i] {
		case "--seeded":
			if seeded != "" {
				return fail(2, fmt.Errorf("duplicate seeded set"))
			}
			seeded = args[i+1]
		case "--adjudicated":
			adjudicated = append(adjudicated, args[i+1])
		case "--out":
			if out != "" {
				return fail(2, fmt.Errorf("duplicate output"))
			}
			out = args[i+1]
		default:
			return fail(2, fmt.Errorf("unknown calibration option %s", args[i]))
		}
	}
	if seeded == "" || out == "" || len(adjudicated) == 0 {
		return fail(2, fmt.Errorf("calibrate requires --seeded, --adjudicated and --out"))
	}
	inputs := append([]string{seeded}, adjudicated...)
	sets := make([]classify.CalibrationSet, 0, len(inputs))
	for _, input := range inputs {
		s, err := loadCalibrationSet(ctx, args[0], args[1], input, load)
		if err != nil {
			return fail(1, err)
		}
		sets = append(sets, s)
	}
	report, err := classify.Calibrate(sets[0], sets[1:])
	if err != nil {
		return fail(1, err)
	}
	if err = classify.WriteCalibration(out, report); err != nil {
		return fail(1, err)
	}
	if err = json.NewEncoder(stdout).Encode(report); err != nil {
		return fail(1, fmt.Errorf("write calibration report: %w", err))
	}
	return 0
}

func loadCalibrationSet(ctx context.Context, cc, rel, input string, load func(context.Context, string, string, taxonomy.Labels) (classificationEvidence, error)) (classify.CalibrationSet, error) {
	var s classify.CalibrationSet
	parts := strings.Split(input, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return s, fmt.Errorf("calibration set must be LABELS:FAMILIES:ARTICLES")
	}
	labels, labelsHash, err := classify.ReadLabels(parts[0])
	if err != nil {
		return s, err
	}
	evidence, err := load(ctx, cc, rel, labels)
	if err != nil {
		return s, err
	}
	families, familiesHash, err := classify.ReadFamilyArtifactForScore(parts[1], evidence.taxonomyHash, evidence.nodes)
	if err != nil {
		return s, err
	}
	articles, err := classify.ReadArticleArtifactForScore(parts[2], evidence.taxonomyHash, familiesHash)
	if err != nil {
		return s, err
	}
	if articles.Country != cc || articles.Release != rel || articles.LabelsHash != labelsHash || articles.Recipe.Article.SourceRecipeHash != families.RecipeHash {
		return s, fmt.Errorf("calibration artifact differs from target release, labels or document recipe")
	}
	b, err := os.ReadFile(parts[2])
	if err != nil {
		return s, fmt.Errorf("read articles artifact: %w", err)
	}
	h := sha256.Sum256(b)
	return classify.CalibrationSet{Labels: labels, Families: families, Articles: articles, Release: evidence.articles, Nodes: evidence.nodes, LabelsHash: labelsHash, FamiliesHash: familiesHash, ArticlesHash: hex.EncodeToString(h[:])}, nil
}
