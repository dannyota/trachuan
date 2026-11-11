// Package app wires the server command and its dependencies.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/embedcache"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	httpReadHeaderTimeout = 5 * time.Second
	httpMaxHeaderBytes    = 16 << 10
	httpReadTimeout       = 10 * time.Second
	httpWriteTimeout      = 30 * time.Second
	httpIdleTimeout       = 120 * time.Second
	authQueryDeadline     = 2 * time.Second
)

// Run executes a server subcommand and returns its exit status.
func Run(ctx context.Context, args []string, stderr io.Writer) int {
	return exitCode(run(ctx, args, os.Stdout, stderr), stderr)
}

func exitCode(err error, stderr io.Writer) int {
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "server:", err)
		return 1
	}
	return 0
}
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing subcommand")
	}
	if args[0] == "migrate" || args[0] == "admin" {
		return runNonServe(ctx, args, stdout)
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if args[0] == "serve" && (cfg.googleID == "" || cfg.googleSecret == "") {
		return errors.New("invalid or missing server configuration")
	}
	pool, err := pgxpool.New(ctx, cfg.authURL)
	if err != nil {
		return errors.New("create auth pool failed")
	}
	defer pool.Close()
	queries := store.New(pool)
	switch args[0] {
	case "admin":
		return runAdmin(ctx, args[1:], credentialStore{Queries: queries, db: pool}, stdout)
	case "serve":
		if len(args) != 1 {
			return errors.New("invalid serve arguments")
		}
		return serve(ctx, cfg, pool, queries, stderr)
	default:
		return errors.New("unknown subcommand")
	}
}
func readiness(runCtx context.Context, check func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if runCtx.Err() != nil || check(r.Context()) != nil || runCtx.Err() != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
func readyCheck(ctx context.Context, auth, activeRelease func(context.Context) error) error {
	authCtx, cancel := context.WithTimeout(ctx, authQueryDeadline)
	err := auth(authCtx)
	if err == nil {
		err = authCtx.Err()
	}
	cancel()
	if err != nil {
		return err
	}
	return activeRelease(ctx)
}

func serve(ctx context.Context, cfg config, pool *pgxpool.Pool, queries *store.Queries, stderr io.Writer) error {
	clock := time.Now
	caches := mcpapi.NewCaches(clock)
	vectors := embedcache.NewVectors(clock)
	client := embedding.NewClient(cfg.embedderURL, &http.Client{Timeout: 2 * time.Second})
	resolver := &release.Resolver{Auth: activeStore{queries}, URLTemplate: cfg.corpusURL, Clock: clock,
		EmbedderFor: func(hash string) retrieval.Embedder {
			return &cachedQuery{query: embedding.Query{Client: client, RecipeHash: hash}, vectors: vectors}
		},
		OnActivate: caches.ActivateRelease}
	verifier := &credential.Verifier{Store: queries, Audience: cfg.audience}
	limiter := limits.New(clock)
	tools := mcpapi.Config{Resolver: mcpResolver{resolver}, Limiter: limiter, Logger: slog.New(slog.NewJSONHandler(stderr, nil)), Caches: caches}
	bounded := credentialStore{Queries: queries, db: pool}
	sessions := &websession.Manager{Store: bounded, PublicURL: cfg.publicURL, Clock: clock}
	login, err := weblogin.New(ctx, weblogin.Config{ClientID: cfg.googleID, ClientSecret: cfg.googleSecret, PublicURL: cfg.publicURL, Store: loginStore{queries}, Sessions: sessions})
	if err != nil {
		resolver.Close()
		return errors.New("google provider discovery failed")
	}
	mux := newWebMux(webDeps{store: bounded, sessions: sessions, login: login, verifier: verifier, tools: tools, loginLimit: newLoginLimiter(clock, cfg.trustedProxies)})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.Handle("GET /readyz", readiness(ctx, func(ctx context.Context) error {
		return readyCheck(ctx, pool.Ping, func(ctx context.Context) error {
			lease, err := resolver.Resolve(ctx, "vn", "")
			if err == nil {
				lease.Done()
			}
			return err
		})
	}))
	server := newHTTPServer(cfg.listen, mux)
	purgeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); purgeLoop(purgeCtx, queries, clock, tools.Logger) }()
	defer func() { cancel(); <-done }()
	return runHTTP(ctx, server, resolver.Close)
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: httpReadHeaderTimeout, MaxHeaderBytes: httpMaxHeaderBytes, ReadTimeout: httpReadTimeout, WriteTimeout: httpWriteTimeout, IdleTimeout: httpIdleTimeout}
}

type httpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

func runHTTP(ctx context.Context, server httpServer, closeResolver func()) error {
	defer closeResolver()
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("server failed")
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return errors.New("server shutdown timed out")
		}
		return nil
	}
}

func runNonServe(ctx context.Context, args []string, stdout io.Writer) error {
	authURL := os.Getenv("SERVER_AUTH_DATABASE_URL")
	if authURL == "" {
		return errors.New("invalid or missing server configuration")
	}
	if args[0] == "migrate" {
		if len(args) != 1 {
			return errors.New("invalid migrate arguments")
		}
		if err := migrate.Up(ctx, authURL); err != nil {
			return errors.New("migration failed")
		}
		return nil
	}
	pool, err := pgxpool.New(ctx, authURL)
	if err != nil {
		return errors.New("create auth pool failed")
	}
	defer pool.Close()
	return runAdmin(ctx, args[1:], credentialStore{Queries: store.New(pool), db: pool}, stdout)
}
