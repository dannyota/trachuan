package releasecheck

import (
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"slices"
	"strings"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FrameworksReport struct {
	SchemaVersions          []int  `json:"schema_versions"`
	RetrievalRecipeVersions []int  `json:"retrieval_recipe_versions"`
	LexicalRecipeVersions   []int  `json:"lexical_recipe_versions"`
	QueryRecipeHash         string `json:"query_recipe_hash"`
}

func DescribeFrameworks(ctx context.Context, client *embedding.Client) (FrameworksReport, error) {
	running, err := client.Recipe(ctx)
	if err != nil {
		return FrameworksReport{}, errors.New("read frameworks query recipe failed")
	}
	return FrameworksReport{schema.SupportedSchemaVersions(), serve.SupportedRetrievalRecipeVersions(), []int{lexical.Recipe1().Version}, running.RecipeHash}, nil
}

func ValidFrameworksDatabase(name string) bool {
	return regexp.MustCompile(`^frameworks_[0-9]{8}(_[0-9]{1,2})?$`).MatchString(name)
}

func validFrameworksIdentity(name, rel string, version int) bool {
	return ValidFrameworksDatabase(name) && name == "frameworks_"+rel && slices.Contains(schema.SupportedSchemaVersions(), version)
}

func frameworksDatabaseConfig(template, name string) (*pgxpool.Config, error) {
	if !ValidFrameworksDatabase(name) || strings.Count(template, "{database}") != 1 {
		return nil, errors.New("invalid frameworks database configuration")
	}
	cfg, err := pgxpool.ParseConfig(strings.Replace(template, "{database}", name, 1))
	if err != nil || cfg.ConnConfig.User != "corpus_reader" || cfg.ConnConfig.Database != name {
		return nil, errors.New("frameworks release-check requires corpus_reader and the target database")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 2
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error { return schema.CheckLogSettings(ctx, conn) }
	return cfg, nil
}

type FrameworksSmoke struct {
	Status     string `json:"status"`
	SearchHits int    `json:"search_hits"`
	ReadItems  int    `json:"read_items"`
}

func CheckFrameworks(ctx context.Context, template, name string, client *embedding.Client) (FrameworksSmoke, error) {
	cfg, err := frameworksDatabaseConfig(template, name)
	if err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("configuration", err)
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("create reader pool", err)
	}
	defer db.Close()
	return checkFrameworksDatabase(ctx, db, name, client)
}

func checkFrameworksDatabase(ctx context.Context, db *pgxpool.Pool, name string, client *embedding.Client) (FrameworksSmoke, error) {
	conn, err := db.Acquire(ctx)
	if err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("connect reader", err)
	}
	err = schema.CheckLogSettings(ctx, conn)
	conn.Release()
	if err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("log settings", err)
	}
	var rel string
	var version int
	if err = db.QueryRow(ctx, `SELECT rel,schema_version FROM release WHERE singleton`).Scan(&rel, &version); err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("release row", err)
	}
	if !validFrameworksIdentity(name, rel, version) {
		return FrameworksSmoke{}, frameworksCheckFailure("release identity or schema", serve.ErrUnavailable)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("create smoke key", err)
	}
	query := embedding.QueryRecipe1()
	engine, err := serve.Open(ctx, db, embedding.Query{Client: client, RecipeHash: query.Hash(), ExpectedRecipe: &query}, serve.Options{CursorKey: key})
	if err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("open engine", err)
	}
	if "frameworks_"+engine.Release() != name {
		return FrameworksSmoke{}, frameworksCheckFailure("engine release identity", serve.ErrUnavailable)
	}
	caller := serve.Caller{Readable: func(string, string) bool { return true }}
	search, err := engine.Search(ctx, serve.Request{Query: "control security", Limit: 1}, caller)
	if err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("search", err)
	}
	if len(search.Hits) != 1 {
		return FrameworksSmoke{}, frameworksCheckFailure("search hits", serve.ErrUnavailable)
	}
	read, err := engine.Read(ctx, serve.ReadRequest{Release: engine.Release(), Items: []serve.ReadItem{{Handle: search.Hits[0].Handle}}}, caller)
	if err != nil {
		return FrameworksSmoke{}, frameworksCheckFailure("read", err)
	}
	if len(read.Items) != 1 || read.Items[0].Error != "" || read.Items[0].NotRead != "" || len(read.Items[0].Units) == 0 {
		return FrameworksSmoke{}, frameworksCheckFailure("read result", serve.ErrUnavailable)
	}
	return FrameworksSmoke{Status: "ok", SearchHits: len(search.Hits), ReadItems: len(read.Items)}, nil
}
