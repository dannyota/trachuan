package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	lawbuild "danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/internal/frameworks/build"
	"github.com/jackc/pgx/v5"
)

type frameworkDatabase struct{ cfg lawbuild.Config }

func newFrameworkDatabase() *frameworkDatabase {
	template, dump := os.Getenv("CORPUS_BUILDER_TEMPLATE"), os.Getenv("CORPUS_PG_DUMP")
	if template == "" {
		template = "corpus_template"
	}
	if dump == "" {
		dump = "pg_dump"
	}
	return &frameworkDatabase{cfg: lawbuild.Config{BuilderURL: os.Getenv("CORPUS_BUILDER_DATABASE_URL"), Template: template, PgDump: dump}}
}

type frameworkDatabaseMarker struct {
	WorkDir string `json:"work_dir"`
	Rel     string `json:"rel"`
	RunID   string `json:"run_id"`
}

func (m frameworkDatabaseMarker) owns(raw string) bool {
	var actual frameworkDatabaseMarker
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&actual) != nil {
		return false
	}
	var extra any
	return dec.Decode(&extra) == io.EOF && actual.WorkDir == m.WorkDir && actual.Rel == m.Rel && actual.RunID != ""
}

func (d *frameworkDatabase) Build(ctx context.Context, req build.DatabaseRequest, load func(context.Context, *pgx.Conn) error, publish func() error) error {
	if !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(req.Rel) || req.WorkDir == "" || req.RunID == "" {
		return fmt.Errorf("invalid frameworks database identity")
	}
	if d.cfg.BuilderURL == "" {
		return fmt.Errorf("CORPUS_BUILDER_DATABASE_URL is required")
	}
	if req.Runtime["go"] == "" || req.Runtime["golang.org/x/text"] == "" {
		return fmt.Errorf("missing lexical runtime provenance")
	}
	if err := localFrameworkDatabase(d.cfg.BuilderURL); err != nil {
		return err
	}
	name := "frameworks_" + req.Rel
	marker := frameworkDatabaseMarker{req.WorkDir, req.Rel, req.RunID}
	raw, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("encode database marker: %w", err)
	}
	return lawbuild.WithDatabase(ctx, d.cfg, name, string(raw), marker.owns, func(ctx context.Context, conn *pgx.Conn) error {
		if err := load(ctx, conn); err != nil {
			return err
		}
		if err := lawbuild.DumpDatabase(ctx, d.cfg, name, req.DumpPath); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return publish()
	})
}

func localFrameworkDatabase(raw string) error {
	u, parseErr := url.Parse(raw)
	if parseErr != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return fmt.Errorf("invalid builder database URL")
	}
	config, err := pgx.ParseConfig(raw)
	if err != nil {
		return fmt.Errorf("invalid builder database URL")
	}
	hosts := []string{config.Host}
	for _, fallback := range config.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	for _, host := range hosts {
		if host == "localhost" || filepath.IsAbs(host) {
			continue
		}
		ip, err := netip.ParseAddr(host)
		if err != nil || !ip.IsLoopback() {
			return fmt.Errorf("builder database must use loopback or a local socket")
		}
	}
	return nil
}
