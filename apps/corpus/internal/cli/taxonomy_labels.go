package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"

	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runTaxonomyLabels(ctx context.Context, args []string, stderr io.Writer) int {
	return runTaxonomyLabelsWith(ctx, args, stderr, loadSeededArticles)
}

func runTaxonomyLabelsWith(ctx context.Context, args []string, stderr io.Writer, load func(context.Context, string, string) ([]taxonomy.LabelArticle, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if len(args) < 2 {
		return fail(2, fmt.Errorf("usage: corpus taxonomy labels <cc> <rel> --out FILE [--seed N]"))
	}
	if _, err := build.DatabaseName(args[0], args[1]); err != nil {
		return fail(2, err)
	}
	opts, err := options(args[2:], map[string]bool{"--out": true, "--seed": true})
	if err != nil {
		return fail(2, err)
	}
	if opts["--out"] == "" {
		return fail(2, fmt.Errorf("--out FILE is required"))
	}
	seed := int64(1)
	if value, ok := opts["--seed"]; ok {
		seed, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fail(2, fmt.Errorf("invalid seed: %w", err))
		}
	}
	pool, err := load(ctx, args[0], args[1])
	if err != nil {
		return fail(1, err)
	}
	articles, err := taxonomy.DrawLabels(pool, seed)
	if err != nil {
		return fail(1, err)
	}
	data, err := json.Marshal(taxonomy.Labels{Country: args[0], Release: args[1], Seed: seed, DrawRule: taxonomy.DrawRuleVersion, Articles: articles})
	if err != nil {
		return fail(1, fmt.Errorf("encode labels: %w", err))
	}
	file, err := os.OpenFile(opts["--out"], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fail(1, fmt.Errorf("create labels: %w", err))
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(opts["--out"])
		if writeErr != nil {
			return fail(1, fmt.Errorf("write labels: %w", writeErr))
		}
		return fail(1, fmt.Errorf("close labels: %w", closeErr))
	}
	return 0
}

func loadSeededArticles(ctx context.Context, cc, rel string) ([]taxonomy.LabelArticle, error) {
	name, err := build.DatabaseName(cc, rel)
	if err != nil {
		return nil, err
	}
	url := os.Getenv("CORPUS_BUILDER_DATABASE_URL")
	if url == "" {
		return nil, fmt.Errorf("CORPUS_BUILDER_DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	cfg.ConnConfig.Database = name
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open release database: %w", err)
	}
	defer db.Close()
	var country, release string
	if err = db.QueryRow(ctx, `SELECT country, rel FROM release WHERE singleton`).Scan(&country, &release); err != nil {
		return nil, fmt.Errorf("read release identity: %w", err)
	}
	if country != cc || release != rel {
		return nil, fmt.Errorf("release identity mismatch")
	}
	return seededArticles(ctx, db)
}

func seededArticles(ctx context.Context, db *pgxpool.Pool) ([]taxonomy.LabelArticle, error) {
	rows, err := db.Query(ctx, `SELECT d.key, p.anchor, p.text, array_agg(DISTINCT n.code ORDER BY n.code) FROM taxonomy_assignment a JOIN provision p ON p.id = a.provision_id JOIN document d ON d.id = p.document_id JOIN taxonomy_node n ON n.id = a.node_id WHERE a.method = 'phapdien_seed' AND p.kind = 'dieu' AND d.class = 'primary' GROUP BY d.key, p.anchor, p.text ORDER BY d.key, p.anchor`)
	if err != nil {
		return nil, fmt.Errorf("read seeded articles: %w", err)
	}
	defer rows.Close()
	var articles []taxonomy.LabelArticle
	for rows.Next() {
		var key, anchor, text string
		var codes []string
		if err = rows.Scan(&key, &anchor, &text, &codes); err != nil {
			return nil, fmt.Errorf("scan seeded article: %w", err)
		}
		articles = append(articles, seededArticle(key, anchor, text, codes))
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read seeded articles: %w", err)
	}
	return articles, nil
}

func seededArticle(key, anchor, text string, codes []string) taxonomy.LabelArticle {
	codes = slices.Clone(codes)
	slices.Sort(codes)
	return taxonomy.LabelArticle{Key: key, Anchor: anchor, OwnTextSHA256: taxonomy.OwnTextSHA256(text), Codes: slices.Compact(codes)}
}
