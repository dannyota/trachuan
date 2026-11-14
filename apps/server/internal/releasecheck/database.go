package releasecheck

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ValidDatabase(name string) bool {
	return regexp.MustCompile(`^corpus_vn_[0-9]{8}(_[0-9]+)?$`).MatchString(name)
}

func databaseConfig(template, name string) (*pgxpool.Config, error) {
	if !ValidDatabase(name) || strings.Count(template, "{database}") != 1 {
		return nil, errors.New("invalid corpus database configuration")
	}
	cfg, err := pgxpool.ParseConfig(strings.Replace(template, "{database}", name, 1))
	if err != nil || cfg.ConnConfig.User != "corpus_reader" || cfg.ConnConfig.Database != name {
		return nil, errors.New("release-check requires corpus_reader and the target database")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 2
	return cfg, nil
}

func Check(ctx context.Context, template, name string, client *embedding.Client) error {
	cfg, err := databaseConfig(template, name)
	if err != nil {
		return err
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("create corpus reader pool failed")
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		return errors.New("connect to corpus as corpus_reader failed")
	}
	return checkDatabase(ctx, db, name, client)
}

func checkDatabase(ctx context.Context, db *pgxpool.Pool, name string, client *embedding.Client) error {
	r, err := retrieval.ReadRelease(ctx, db)
	if err != nil {
		return fmt.Errorf("load target release: %w", err)
	}
	if "corpus_"+r.Country+"_"+r.Rel != name || r.DenseRecipe.QueryHash != embedding.QueryRecipe1().Hash() {
		return errors.New("incompatible target release identity or query recipe")
	}
	query := embedding.Query{Client: client, RecipeHash: r.DenseRecipe.QueryHash, ExpectedRecipe: &r.DenseRecipe.Query}
	engine, err := retrieval.Open(ctx, db, query, retrieval.Options{Now: time.Now, Profile: "serving"})
	if err != nil {
		return fmt.Errorf("open target retrieval engine: %w", err)
	}
	defer engine.Retire()
	if _, err = engine.Search(ctx, retrieval.Request{Query: "quyền và nghĩa vụ", Limit: 1}); err != nil {
		return fmt.Errorf("search target release: %w", err)
	}
	var key string
	if err = db.QueryRow(ctx, `SELECT key FROM document ORDER BY id LIMIT 1`).Scan(&key); err != nil {
		return fmt.Errorf("select smoke read document: %w", err)
	}
	read, err := engine.Read(ctx, retrieval.ReadRequest{Release: r.Rel, Items: []retrieval.ReadItem{{Handle: key}}})
	if err != nil {
		return fmt.Errorf("read target release: %w", err)
	}
	if len(read.Items) != 1 || read.Items[0].Error != "" || read.Items[0].NotRead != "" || read.Items[0].Document == nil {
		return errors.New("target smoke read returned no document")
	}
	return nil
}
