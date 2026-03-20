package main

import (
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
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/middleware"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/txlock"
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
	handler := serverledger.NewHandler(hl, lock, *dataDir)
	mux := http.NewServeMux()
	path, svcHandler := floatv1connect.NewLedgerServiceHandler(
		handler,
		connect.WithInterceptors(middleware.NewLoggingInterceptor(logger)),
	)
	mux.Handle(path, svcHandler)

	slog.Info("floatd listening", "addr", listenAddr)
	if err := http.ListenAndServe(listenAddr, h2c.NewHandler(mux, &http2.Server{})); err != nil {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
