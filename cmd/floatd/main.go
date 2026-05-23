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
	"github.com/brendanv/float/internal/tsnetsrv"
	"github.com/brendanv/float/internal/txlock"
	"github.com/brendanv/float/internal/webhooks"
	"github.com/brendanv/float/internal/webui"
)

// stripeWebhookAdapter bridges the webhooks.StripeImporter interface to the
// ledger handler so internal/webhooks doesn't need to depend on the connectrpc
// handler package.
type stripeWebhookAdapter struct {
	h *serverledger.Handler
}

func (a *stripeWebhookAdapter) ImportRefreshedAccount(ctx context.Context, stripeAccountID string) (int, error) {
	return a.h.WebhookImportStripeAccount(ctx, stripeAccountID)
}

func (a *stripeWebhookAdapter) AccountDisconnected(ctx context.Context, stripeAccountID string) error {
	return a.h.WebhookMarkStripeAccountDisconnected(ctx, stripeAccountID)
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

	tsSrv, err := tsnetsrv.New(ctx, cfg.Tailscale, *dataDir, logger)
	if err != nil {
		slog.Error("tsnet init", "error", err)
		os.Exit(1)
	}
	if tsSrv != nil {
		secret := cfg.Webhooks.StripeSigningSecret
		if env := os.Getenv("STRIPE_WEBHOOK_SECRET"); env != "" {
			secret = env
		}
		if secret != "" {
			tsSrv.Mux.Handle("/webhooks/stripe", webhooks.NewStripeHandler(secret, &stripeWebhookAdapter{h: handler}, logger))
			tsSrv.Mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok\n"))
			})
			slog.Info("stripe webhook listener mounted", "path", "/webhooks/stripe")
		} else {
			slog.Warn("tailscale enabled but stripe webhook secret not configured; webhook listener will not accept events")
		}
		go func() {
			if err := tsSrv.Serve(ctx); err != nil {
				slog.Error("tsnet serve", "error", err)
			}
		}()
	}

	httpSrv := &http.Server{Addr: listenAddr, Handler: h2c.NewHandler(mux, &http2.Server{})}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
		if tsSrv != nil {
			_ = tsSrv.Shutdown(context.Background())
		}
	}()

	slog.Info("floatd listening", "addr", listenAddr, "webui", true)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
