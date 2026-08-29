package middleware

import (
	"context"
	"errors"
	"strconv"

	"connectrpc.com/connect"
)

// GenerationHeader carries the current txlock generation on every response.
const GenerationHeader = "X-Float-Generation"

// NewGenerationInterceptor returns a ConnectRPC unary interceptor that stamps
// the current txlock generation on every response.
//
// The web client uses this to know which cube URL to fetch. Piggybacking on
// responses the client already makes means a write performed anywhere is
// noticed by the next RPC, with no polling and no extra round trip. The header
// is set on error responses too, since a failed write still bumps the
// generation.
func NewGenerationInterceptor(gen func() uint64) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			value := strconv.FormatUint(gen(), 10)
			if err != nil {
				// On the error path Connect carries headers on the error
				// itself; resp is nil.
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					connectErr.Meta().Set(GenerationHeader, value)
				}
				return resp, err
			}
			resp.Header().Set(GenerationHeader, value)
			return resp, nil
		}
	}
}
