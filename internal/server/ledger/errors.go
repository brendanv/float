package ledger

import (
	"context"

	"connectrpc.com/connect"
	"github.com/brendanv/float/internal/slogctx"
)

// rpcErr logs msg at error level (including err and any extra attrs) then
// returns a ConnectRPC CodeInternal error wrapping err. Centralises the
// log-then-return pattern that appears throughout the handler methods.
func rpcErr(ctx context.Context, err error, msg string, attrs ...any) error {
	slogctx.FromContext(ctx).ErrorContext(ctx, msg, append(attrs, "error", err)...)
	return connect.NewError(connect.CodeInternal, err)
}
