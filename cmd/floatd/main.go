package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"connectrpc.com/connect"
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

	var logLevel slog.LevelVar // defaults to Info
	if *verbose {
		logLevel.Set(slog.LevelDebug)
	}
	broadcaster := logstream.NewBroadcaster()
	jsonHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &logLevel})
	logger := slog.New(logstream.NewBroadcastHandler(jsonHandler, broadcaster))
	slog.SetDefault(logger)

	if *dataDir == "" {
		slog.Error("--data-dir is required")
		os.Exit(1)
	}

	cfg, err := config.Load(filepath.Join(*dataDir, "config.toml"))
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	listenAddr := *addr
	if listenAddr == "" {
		port := cfg.Server.Port
		if port == 0 {
			port = 8080
		}
		listenAddr = fmt.Sprintf(":%d", port)
	}

	mainJournal := filepath.Join(*dataDir, "main.journal")
	if _, err := os.Stat(mainJournal); os.IsNotExist(err) {
		slog.Info("main.journal not found, creating empty file", "path", mainJournal)
		if err := os.WriteFile(mainJournal, nil, 0644); err != nil {
			slog.Error("create main.journal", "error", err)
			os.Exit(1)
		}
	}

	hl, err := hledger.New("hledger", mainJournal)
	if err != nil {
		slog.Error("hledger init", "error", err)
		os.Exit(1)
	}
	hl.SetConcurrency(cfg.Server.HledgerConcurrency)

	lock := txlock.New(*dataDir, hl)

	snap, err := gitsnap.New(*dataDir)
	if err != nil {
		slog.Error("gitsnap init", "error", err)
		os.Exit(1)
	}
	if recoverErr := snap.RecoverUncommitted(context.Background()); recoverErr != nil {
		slog.Warn("gitsnap: recover uncommitted", "error", recoverErr)
	}
	lock.SetSnap(snap)

	var backfillCount, stripeMigrateCount, declaredCount int
	var commodityChanged, accountsFileChanged, accountsIncludeChanged bool
	if err := lock.Do(context.Background(), "startup migrations", func() error {
		var err error

		backfillCount, err = journal.MigrateFIDs(*dataDir)
		if err != nil {
			return fmt.Errorf("fid backfill: %w", err)
		}

		stripeMigrateCount, err = journal.MigrateStripeTxnTag(context.Background(), hl, *dataDir)
		if err != nil {
			return fmt.Errorf("stripe-txn tag migration: %w", err)
		}

		commodityChanged, err = journal.EnsureCommodityDirective(*dataDir, "USD")
		if err != nil {
			return fmt.Errorf("commodity directive: %w", err)
		}

		accountsFileChanged, err = journal.EnsureAccountsFile(*dataDir)
		if err != nil {
			return fmt.Errorf("account declarations startup: %w", err)
		}
		accountsIncludeChanged, err = journal.EnsureAccountsInclude(*dataDir)
		if err != nil {
			return fmt.Errorf("account declarations startup: %w", err)
		}
		undeclared, err := hl.UndeclaredAccounts(context.Background())
		if err != nil {
			return fmt.Errorf("account declarations startup: %w", err)
		}
		for _, name := range undeclared {
			if err := journal.AppendAccountDeclaration(*dataDir, name); err != nil {
				return fmt.Errorf("account declarations startup: %w", err)
			}
		}
		declaredCount = len(undeclared)

		changed := backfillCount > 0 || stripeMigrateCount > 0 || commodityChanged ||
			accountsFileChanged || accountsIncludeChanged || declaredCount > 0
		if !changed {
			return txlock.ErrNoChanges
		}
		return nil
	}); err != nil {
		slog.Error("startup migrations", "error", err)
		os.Exit(1)
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

	authn := auth.New(os.Getenv("FLOAT_AUTH_PASSPHRASE"))
	if authn.Enabled() {
		slog.Info("auth enabled (FLOAT_AUTH_PASSPHRASE is set)")
	} else {
		slog.Warn("auth disabled: set FLOAT_AUTH_PASSPHRASE to require a passphrase")
	}

	c := cache.New[any](lock.Generation)
	handler := serverledger.NewHandler(hl, lock, *dataDir, filepath.Join(*dataDir, "config.toml"), c, snap, cfg, broadcaster)

	warmer := warm.New(lock.Generation, handler.WarmEntries, warmDebounce)
	lock.OnCommit(warmer.Trigger)
	lock.OnCommit(newStatsLogger(c, hl))

	mux := http.NewServeMux()
	path, svcHandler := floatv1connect.NewLedgerServiceHandler(
		handler,
		connect.WithInterceptors(
			middleware.NewLoggingInterceptor(logger),
			auth.NewServerInterceptor(authn),
		),
	)
	mux.Handle(path, svcHandler)
	mux.Handle("/api/auth", authn.StatusHandler())
	mux.Handle("/api/login", authn.LoginHandler())
	mux.Handle("/api/logout", authn.LogoutHandler())
	mux.Handle("/", webui.Handler())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go handler.StartDailyStripeImport(ctx)

	httpSrv := &http.Server{Addr: listenAddr, Handler: h2c.NewHandler(mux, &http2.Server{})}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		slog.Error("listen", "error", err)
		os.Exit(1)
	}
	// Kick the startup warm pass only once the listener is bound, so warming
	// never delays accepting connections.
	warmer.Start(ctx)

	slog.Info("floatd listening", "addr", listenAddr, "webui", true)
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
	// ListenAndServe returns ErrServerClosed as soon as Shutdown is initiated;
	// wait for Shutdown to finish draining in-flight requests before exiting,
	// otherwise a SIGTERM mid-import kills handlers inside txlock with partial
	// journal writes on disk.
	<-shutdownDone
	slog.Info("floatd shut down")
}
