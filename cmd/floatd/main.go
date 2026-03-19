package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
)

func main() {
	dataDir := flag.String("data-dir", "", "path to float data directory (required)")
	addr := flag.String("addr", "", "listen address (overrides config; default :8080)")
	flag.Parse()

	if *dataDir == "" {
		log.Fatal("--data-dir is required")
	}

	cfg, err := config.Load(filepath.Join(*dataDir, "config.toml"))
	if err != nil {
		log.Fatalf("load config: %v", err)
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
		log.Fatalf("hledger init: %v", err)
	}

	handler := serverledger.NewHandler(hl)
	mux := http.NewServeMux()
	path, svcHandler := floatv1connect.NewLedgerServiceHandler(handler)
	mux.Handle(path, svcHandler)

	log.Printf("floatd listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, h2c.NewHandler(mux, &http2.Server{})); err != nil {
		log.Fatalf("server: %v", err)
	}
}
