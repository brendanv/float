package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/net/http2"

	"github.com/brendanv/float/cmd/float/ui"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

func main() {
	server := flag.String("server", "localhost:8080", "floatd address (host:port)")
	flag.Parse()

	// HTTP/2 client with h2c (plain HTTP, no TLS).
	plainClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}

	client := floatv1connect.NewLedgerServiceClient(
		plainClient,
		"http://"+*server,
	)

	model := ui.New(client)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
