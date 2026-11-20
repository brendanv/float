package mcpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// InProcess serves h over in-memory connections, so MCP tool calls run
// through the real Connect handler — interceptors, validation, streaming —
// without a network round trip. Use Client to reach it and Shutdown to stop
// it once nothing calls it any more.
type InProcess struct {
	ln  *memListener
	srv *http.Server
}

// baseURL is the URL clients of an InProcess server use; the host is never
// resolved.
const baseURL = "http://floatd.inprocess"

// NewInProcess starts serving h.
func NewInProcess(h http.Handler) *InProcess {
	ln := &memListener{conns: make(chan net.Conn), closed: make(chan struct{})}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return &InProcess{ln: ln, srv: srv}
}

// Client returns an HTTP client whose connections go to the in-process
// server, whatever the request URL.
func (p *InProcess) Client() *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext:       p.ln.dial,
		ForceAttemptHTTP2: false,
		MaxIdleConns:      16,
		IdleConnTimeout:   90 * time.Second,
	}}
}

// LedgerClient returns a LedgerService client for the in-process server.
func (p *InProcess) LedgerClient() floatv1connect.LedgerServiceClient {
	return floatv1connect.NewLedgerServiceClient(p.Client(), baseURL)
}

// Shutdown stops the server, waiting for in-flight requests like
// http.Server.Shutdown.
func (p *InProcess) Shutdown(ctx context.Context) error {
	return p.srv.Shutdown(ctx)
}

// memListener is a net.Listener whose connections are net.Pipe pairs handed
// over by dial.
type memListener struct {
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func (l *memListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *memListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *memListener) Addr() net.Addr { return memAddr{} }

func (l *memListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	server, client := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.closed:
		_, _ = server.Close(), client.Close()
		return nil, errors.New("in-process server closed")
	case <-ctx.Done():
		_, _ = server.Close(), client.Close()
		return nil, ctx.Err()
	}
}

type memAddr struct{}

func (memAddr) Network() string { return "memory" }
func (memAddr) String() string  { return "inprocess" }
