package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
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
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &logLevel}))
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

	hl, err := hledger.New("hledger", filepath.Join(*dataDir, "main.journal"))
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

	c := cache.New[any](lock.Generation)
	handler := serverledger.NewHandler(hl, lock, *dataDir, c, snap, cfg)
	mux := http.NewServeMux()
	path, svcHandler := floatv1connect.NewLedgerServiceHandler(
		handler,
		connect.WithInterceptors(middleware.NewLoggingInterceptor(logger)),
	)
	mux.Handle(path, svcHandler)
	mux.Handle("/", webui.Handler())

	slog.Info("floatd listening", "addr", listenAddr, "webui", true)
	if err := http.ListenAndServe(listenAddr, h2c.NewHandler(mux, &http2.Server{})); err != nil {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
