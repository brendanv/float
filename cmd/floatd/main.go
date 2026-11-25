package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/auth"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/logstream"
	"github.com/brendanv/float/internal/middleware"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/server/mcpserver"
	"github.com/brendanv/float/internal/txlock"
	"github.com/brendanv/float/internal/warm"
	"github.com/brendanv/float/internal/webui"
)

// warmDebounce coalesces bursts of writes (imports, apply-rules) into a
// single warm pass after the burst settles, instead of one pass per write.
const warmDebounce = 500 * time.Millisecond

// newStatsLogger returns a txlock.CommitHook that logs cache hit/miss/load
// and hledger invocation counts accrued since the previous generation, at
// Info level (so it reaches the StreamLogs RPC). txlock serializes commits
// via its internal mutex, so the closure's plain counters are never accessed
// concurrently.
func newStatsLogger(c *cache.Cache[any], hl *hledger.Client) txlock.CommitHook {
	var lastHits, lastMisses, lastLoads, lastInvocations uint64
	return func(gen uint64) {
		hits, misses, loads := c.Stats()
		invocations := hl.Invocations()
		slog.Info("generation stats since previous generation",
			"generation", gen,
			"cache_hits", hits-lastHits,
			"cache_misses", misses-lastMisses,
			"cache_loads", loads-lastLoads,
			"hledger_invocations", invocations-lastInvocations,
		)
		lastHits, lastMisses, lastLoads, lastInvocations = hits, misses, loads, invocations
	}
}

func main() {
	dataDir := flag.String("data-dir", "", "path to float data directory (required)")
	addr := flag.String("addr", "", "listen address (overrides config; default :8080)")
	verbose := flag.Bool("verbose", false, "enable debug-level logging (hledger queries, args, durations)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, options{
		dataDir:    *dataDir,
		addr:       *addr,
		verbose:    *verbose,
		passphrase: os.Getenv("FLOAT_AUTH_PASSPHRASE"),
		logOut:     os.Stderr,
	}); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// options configures run. main fills it from flags and the environment; the
// smoke test fills it directly.
type options struct {
	dataDir    string
	addr       string // listen address; empty uses config's port or :8080
	verbose    bool
	passphrase string    // FLOAT_AUTH_PASSPHRASE; empty disables auth
	logOut     io.Writer // JSON log destination
	// ready, if set, is called with the bound listener address once floatd
	// is about to serve.
	ready func(net.Addr)
}

// run starts floatd and serves until ctx is cancelled, then shuts down
// gracefully. It returns an error if startup fails.
func run(ctx context.Context, opts options) error {
	var logLevel slog.LevelVar // defaults to Info
	if opts.verbose {
		logLevel.Set(slog.LevelDebug)
	}
	broadcaster := logstream.NewBroadcaster()
	jsonHandler := slog.NewJSONHandler(opts.logOut, &slog.HandlerOptions{Level: &logLevel})
	logger := slog.New(logstream.NewBroadcastHandler(jsonHandler, broadcaster))
	slog.SetDefault(logger)

	if opts.dataDir == "" {
		return errors.New("--data-dir is required")
	}

	cfg, err := config.Load(filepath.Join(opts.dataDir, "config.toml"))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	listenAddr := opts.addr
	if listenAddr == "" {
		port := cfg.Server.Port
		if port == 0 {
			port = 8080
		}
		listenAddr = fmt.Sprintf(":%d", port)
	}

	mainJournal := filepath.Join(opts.dataDir, "main.journal")
	if _, err := os.Stat(mainJournal); os.IsNotExist(err) {
		slog.Info("main.journal not found, creating empty file", "path", mainJournal)
		if err := os.WriteFile(mainJournal, nil, 0644); err != nil { //nolint:forbidigo // startup bootstrap, before any txlock exists
			return fmt.Errorf("create main.journal: %w", err)
		}
	}

	hl, err := hledger.New("hledger", mainJournal)
	if err != nil {
		return fmt.Errorf("hledger init: %w", err)
	}
	hl.SetConcurrency(cfg.Server.HledgerConcurrency)

	lock := txlock.New(opts.dataDir, hl)

	snap, err := gitsnap.New(opts.dataDir)
	if err != nil {
		return fmt.Errorf("gitsnap init: %w", err)
	}
	if recoverErr := snap.RecoverUncommitted(ctx); recoverErr != nil {
		slog.Warn("gitsnap: recover uncommitted", "error", recoverErr)
	}
	lock.SetSnap(snap)

	var backfillCount, stripeMigrateCount, declaredCount int
	var commodityChanged, accountsFileChanged, accountsIncludeChanged, migrationsChanged bool
	if err := lock.Do(ctx, "startup migrations", func() error {
		var err error

		backfillCount, err = journal.MigrateFIDs(opts.dataDir)
		if err != nil {
			return fmt.Errorf("fid backfill: %w", err)
		}

		stripeMigrateCount, err = journal.MigrateStripeTxnTag(ctx, hl, opts.dataDir)
		if err != nil {
			return fmt.Errorf("stripe-txn tag migration: %w", err)
		}

		commodityChanged, err = journal.EnsureCommodityDirective(opts.dataDir, "USD")
		if err != nil {
			return fmt.Errorf("commodity directive: %w", err)
		}

		accountsFileChanged, err = journal.EnsureAccountsFile(opts.dataDir)
		if err != nil {
			return fmt.Errorf("account declarations startup: %w", err)
		}
		accountsIncludeChanged, err = journal.EnsureAccountsInclude(opts.dataDir)
		if err != nil {
			return fmt.Errorf("account declarations startup: %w", err)
		}
		undeclared, err := hl.UndeclaredAccounts(ctx)
		if err != nil {
			return fmt.Errorf("account declarations startup: %w", err)
		}
		for _, name := range undeclared {
			if err := journal.AppendAccountDeclaration(opts.dataDir, name); err != nil {
				return fmt.Errorf("account declarations startup: %w", err)
			}
		}
		declaredCount = len(undeclared)

		migrationsChanged = backfillCount > 0 || stripeMigrateCount > 0 || commodityChanged ||
			accountsFileChanged || accountsIncludeChanged || declaredCount > 0
		if !migrationsChanged {
			return txlock.ErrNoChanges
		}
		return nil
	}); err != nil {
		return fmt.Errorf("startup migrations: %w", err)
	}
	if !migrationsChanged {
		// lock.Do's ErrNoChanges fast path skips the generation bump, gitsnap
		// commit, and hledger check that a real migration would have paid
		// for. Run the check directly so a journal hand-edited into an
		// invalid state while floatd was stopped still fails startup loudly,
		// instead of surfacing as opaque per-RPC hledger errors later.
		if err := hl.Check(ctx); err != nil {
			return fmt.Errorf("startup check: %w", err)
		}
	}
	if backfillCount > 0 {
		slog.Info("fid backfill: assigned codes to transactions", "count", backfillCount)
	}
	if stripeMigrateCount > 0 {
		slog.Info("stripe-txn migration: moved tags to float-stripe-txn", "count", stripeMigrateCount)
	}
	if declaredCount > 0 {
		slog.Info("account declarations: declared undeclared accounts", "count", declaredCount)
	}

	authn := auth.New(opts.passphrase)
	if authn.Enabled() {
		slog.Info("auth enabled (FLOAT_AUTH_PASSPHRASE is set)")
	} else {
		slog.Warn("auth disabled: set FLOAT_AUTH_PASSPHRASE to require a passphrase")
	}

	c := cache.New[any](lock.Generation)
	handler := serverledger.NewHandler(hl, lock, opts.dataDir, filepath.Join(opts.dataDir, "config.toml"), c, snap, cfg, broadcaster)

	warmer := warm.New(lock.Generation, handler.WarmEntries, warmDebounce, hl.WaitUncontended)
	lock.OnCommit(warmer.Trigger)
	lock.OnCommit(newStatsLogger(c, hl))
	lock.OnCommit(handler.WakeAccountSuggester)

	mux := http.NewServeMux()
	interceptors := connect.WithInterceptors(
		middleware.NewLoggingInterceptor(logger),
		auth.NewServerInterceptor(authn),
		serverledger.NewUndoInterceptor(),
	)
	path, svcHandler := floatv1connect.NewLedgerServiceHandler(handler, interceptors)
	mux.Handle(path, svcHandler)
	// gRPC server reflection (v1 and v1alpha for older clients) so tools like
	// grpcurl and `buf curl` can discover services without local protos.
	// Gated by the same auth interceptor as the service itself.
	reflector := grpcreflect.NewStaticReflector(floatv1connect.LedgerServiceName)
	mux.Handle(grpcreflect.NewHandlerV1(reflector, interceptors))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, interceptors))
	// MCP endpoint for agents. Tool calls go through an in-process Connect
	// client to svcHandler, so they pass the same interceptors (auth
	// included) as external clients. The in-process server must outlive the
	// public one: it is shut down only when run returns, after httpSrv has
	// drained in-flight /mcp requests.
	inproc := mcpserver.NewInProcess(svcHandler)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = inproc.Shutdown(shutdownCtx)
	}()
	mcpHandler, err := mcpserver.NewHandler(inproc.LedgerClient(), authn.Enabled())
	if err != nil {
		return err
	}
	mux.Handle(mcpserver.Path, authn.RequireBearer(mcpHandler))
	mux.Handle("/api/auth", authn.StatusHandler())
	mux.Handle("/api/login", authn.LoginHandler())
	mux.Handle("/api/logout", authn.LogoutHandler())
	mux.Handle("/", webui.Handler())

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	httpSrv := &http.Server{Addr: listenAddr, Handler: h2c.NewHandler(mux, &http2.Server{})}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		// ctx is already cancelled here; drain with a fresh deadline.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	go handler.StartDailyStripeImport(ctx)
	go handler.StartAccountSuggester(ctx)

	// Kick the startup warm pass only once the listener is bound, so warming
	// never delays accepting connections.
	warmer.Start(ctx)

	slog.Info("floatd listening", "addr", ln.Addr().String(), "webui", true)
	if opts.ready != nil {
		opts.ready(ln.Addr())
	}
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	// ListenAndServe returns ErrServerClosed as soon as Shutdown is initiated;
	// wait for Shutdown to finish draining in-flight requests before exiting,
	// otherwise a SIGTERM mid-import kills handlers inside txlock with partial
	// journal writes on disk.
	<-shutdownDone
	slog.Info("floatd shut down")
	return nil
}
