package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
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
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/logstream"
	"github.com/brendanv/float/internal/middleware"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/txlock"
	"github.com/brendanv/float/internal/webui"
)

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

	var backfillCount int
	if err := lock.Do(context.Background(), "migrate transaction IDs", func() error {
		n, err := journal.MigrateFIDs(*dataDir)
		backfillCount = n
		return err
	}); err != nil {
		slog.Error("fid backfill", "error", err)
		os.Exit(1)
	}
	if backfillCount > 0 {
		slog.Info("fid backfill: assigned codes to transactions", "count", backfillCount)
	}

	var stripeMigrateCount int
	if err := lock.Do(context.Background(), "migrate stripe-txn tags to float-stripe-txn", func() error {
		n, err := journal.MigrateStripeTxnTag(context.Background(), hl, *dataDir)
		stripeMigrateCount = n
		return err
	}); err != nil {
		slog.Error("stripe-txn tag migration", "error", err)
		os.Exit(1)
	}
	if stripeMigrateCount > 0 {
		slog.Info("stripe-txn migration: moved tags to float-stripe-txn", "count", stripeMigrateCount)
	}

	if err := lock.Do(context.Background(), "ensure commodity directive", func() error {
		return journal.EnsureCommodityDirective(*dataDir, "USD")
	}); err != nil {
		slog.Error("commodity directive", "error", err)
		os.Exit(1)
	}

	if err := lock.Do(context.Background(), "declare undeclared accounts", func() error {
		if err := journal.EnsureAccountsFile(*dataDir); err != nil {
			return err
		}
		if err := journal.EnsureAccountsInclude(*dataDir); err != nil {
			return err
		}
		undeclared, err := hl.UndeclaredAccounts(context.Background())
		if err != nil {
			return err
		}
		for _, name := range undeclared {
			if err := journal.AppendAccountDeclaration(*dataDir, name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		slog.Error("account declarations startup", "error", err)
		os.Exit(1)
	}

	c := cache.New[any](lock.Generation)
	handler := serverledger.NewHandler(hl, lock, *dataDir, filepath.Join(*dataDir, "config.toml"), c, snap, cfg, broadcaster)
	mux := http.NewServeMux()
	path, svcHandler := floatv1connect.NewLedgerServiceHandler(
		handler,
		connect.WithInterceptors(middleware.NewLoggingInterceptor(logger)),
	)
	mux.Handle(path, svcHandler)
	mux.Handle("/", webui.Handler())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Server.SSHPort > 0 {
		go startSSHServer(ctx, *dataDir, listenAddr, cfg.Server.SSHPort)
	}

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

	slog.Info("floatd listening", "addr", listenAddr, "webui", true)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
