package ledger

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/slogctx"
)

// rpcErr logs msg at error level (including err and any extra attrs) then
// returns a ConnectRPC error wrapping err. Most failures map to CodeInternal;
// rejected query input (hledger.ErrUnsafeQuery) is the caller's fault and maps
// to CodeInvalidArgument. Centralises the log-then-return pattern that appears
// throughout the handler methods.
func rpcErr(ctx context.Context, err error, msg string, attrs ...any) error {
	slogctx.FromContext(ctx).ErrorContext(ctx, msg, append(attrs, "error", err)...)
	code := connect.CodeInternal
	if errors.Is(err, hledger.ErrUnsafeQuery) {
		code = connect.CodeInvalidArgument
	}
	return connect.NewError(code, err)
}
