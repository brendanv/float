package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	tea "charm.land/bubbletea/v2"
	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	"github.com/brendanv/float/cmd/float/ui"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/auth"
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
	baseURL := "http://" + *server

	// Credential: FLOAT_AUTH_PASSPHRASE (a passphrase is a valid bearer) or a
	// previously saved session token for this server. Empty means none yet;
	// the AuthGate prompts if the server rejects the first request.
	credential := os.Getenv("FLOAT_AUTH_PASSPHRASE")
	if credential == "" {
		credential = ui.LoadAuthToken(*server)
	}
	tokenSrc := auth.NewTokenSource(credential)

	client := floatv1connect.NewLedgerServiceClient(
		plainClient,
		baseURL,
		connect.WithInterceptors(auth.NewClientInterceptor(tokenSrc)),
	)

	probe := func(ctx context.Context) error {
		_, err := client.GetGeneralConfig(ctx, connect.NewRequest(&floatv1.GetGeneralConfigRequest{}))
		return err
	}
	login := func(ctx context.Context, passphrase string) (string, error) {
		return auth.LoginHTTP(ctx, plainClient, baseURL, passphrase)
	}
	onToken := func(token string) {
		tokenSrc.Set(token)
		ui.SaveAuthToken(*server, token)
	}

	model := ui.NewAuthGate(ui.New(client), probe, login, onToken)

	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
