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

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/embedcache"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/oauth"
	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/passwordauth"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/apps/server/internal/seal"
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
	if args[0] == "release-check" {
		return runReleaseCheck(ctx, args[1:], stdout)
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
	if args[0] == "serve" {
		cfg.accountKeys, err = seal.LoadFile(os.Getenv("SERVER_ACCOUNT_KEYS_FILE"))
		if err != nil {
			return err
		}
		cfg.mail, err = loadMailConfig(os.Getenv("SERVER_PUBLIC_URL"), cfg.listen)
		if err != nil {
			return err
		}
	}
	if cfg.googleIssuer != "" && args[0] == "serve" {
		_, _ = fmt.Fprintln(stderr, "Google issuer override active")
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
		EmbedderFor: func(hash string, expected embedding.Recipe) retrieval.Embedder {
			return &cachedQuery{query: embedding.Query{Client: client, RecipeHash: hash, ExpectedRecipe: &expected}, vectors: vectors}
		},
		OnActivate: caches.ActivateRelease, AllowTestRecipe: cfg.strictDeployment,
		OnPostings: func(rel string, outcome retrieval.PostingsOutcome) {
			slog.New(slog.NewJSONHandler(stderr, nil)).Info("sparse postings", "release", rel, "status", outcome.Status, "reason", outcome.Reason, "load_seconds", outcome.LoadSeconds, "bytes", outcome.Bytes, "estimated_peak_bytes", outcome.EstimatedPeakBytes)
		},
		OnTestRecipeRefused: func() { _, _ = fmt.Fprintln(stderr, "test embedding recipe refused") }}
	verifier := &credential.Verifier{Store: queries, Audience: cfg.audience}
	limiter := limits.New(clock)
	reportService := &reports.Service{Store: reports.PGXRunner{DB: pool}, Resolver: reports.ReleaseResolver{Resolver: resolver}, Clock: clock}
	tools := mcpapi.Config{OmitCompactRelated: cfg.omitCompactRelated, Reports: reportService, Resolver: mcpResolver{resolver}, Limiter: limiter, Logger: slog.New(slog.NewJSONHandler(stderr, nil)), Caches: caches}
	bounded := credentialStore{Queries: queries, db: pool}
	sessions := &websession.Manager{Store: bounded, PublicURL: cfg.publicURL, Clock: clock}
	login, err := weblogin.New(ctx, weblogin.Config{ClientID: cfg.googleID, ClientSecret: cfg.googleSecret, PublicURL: cfg.publicURL, Issuer: cfg.googleIssuer, SignupEmails: cfg.signupEmails, Store: loginStore{queries}, Sessions: sessions})
	if err != nil {
		resolver.Close()
		return errors.New("google provider discovery failed")
	}
	hasher := password.NewHasher()
	passwordLogin := &passwordauth.Handler{Store: queries, Proofs: queries, Hasher: hasher, HashPassword: hasher.Hash, Counter: password.NewCounter(clock), Sessions: sessions, PublicURL: cfg.publicURL}
	flows := &accountflow.Service{DB: pool, Ring: cfg.accountKeys, SignupEmails: cfg.signupEmails, Logger: tools.Logger}
	passwordLogin.Flows, passwordLogin.Factors, passwordLogin.Enrollment = flows, flows, cfg.totpEnrollment
	passwordLogin.Evict = func(id int64) { sessions.EvictAccount(id); verifier.EvictAccount(id) }
	mux := newWebMux(webDeps{password: passwordLogin, store: bounded, reportStore: queries, sessions: sessions, login: login, verifier: verifier, tools: tools, loginLimit: newLoginLimiter(clock, cfg.trustedProxies), oauthDB: pool, oauthClients: cfg.oauthClients, publicURL: cfg.publicURL})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.Handle("GET /readyz", readiness(ctx, func(ctx context.Context) error {
		return readyCheck(ctx, pool.Ping, func(ctx context.Context) error {
			lease, resolveErr := resolver.Resolve(ctx, "vn", "")
			if resolveErr == nil {
				lease.Done()
			}
			return resolveErr
		})
	}))
	stopMail, err := startMail(ctx, cfg, queries, stderr)
	if err != nil {
		resolver.Close()
		return err
	}
	defer stopMail()
	server := newHTTPServer(cfg.listen, mux)
	purgeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); purgeLoop(purgeCtx, queries, clock, tools.Logger) }()
	retentionDone := make(chan struct{})
	go func() { defer close(retentionDone); oauth.RunRetention(purgeCtx, queries, tools.Logger) }()
	defer func() { cancel(); <-done; <-retentionDone }()
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
	if len(args) >= 3 && args[1] == "account" && args[2] == "keys" {
		return runAccountKeys(ctx, args[3:], store.New(pool), stdout)
	}
	return runAdmin(ctx, args[1:], credentialStore{Queries: store.New(pool), db: pool}, stdout)
}
