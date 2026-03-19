package middleware

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/brendanv/float/internal/slogctx"
)

// NewLoggingInterceptor returns a ConnectRPC unary interceptor that injects a
// request-scoped slog.Logger into the context and logs each request's start
// and completion (or failure).
//
// The logger is seeded with:
//   - procedure: the fully-qualified RPC name
//   - peer: the client address
//   - request_id: an 8-character UUID prefix for correlation
//
// Downstream handlers retrieve the logger with slogctx.FromContext(ctx) and
// may attach additional fields via logger.With(...).
func NewLoggingInterceptor(base *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			requestID := uuid.NewString()[:8]
			logger := base.With(
				"procedure", req.Spec().Procedure,
				"peer", req.Peer().Addr,
				"request_id", requestID,
			)
			ctx = slogctx.WithLogger(ctx, logger)

			logger.InfoContext(ctx, "request received")

			start := time.Now()
			resp, err := next(ctx, req)
			durationMs := time.Since(start).Milliseconds()

			if err != nil {
				code := connect.CodeInternal
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					code = connectErr.Code()
				}
				logger.ErrorContext(ctx, "request failed",
					"code", code.String(),
					"duration_ms", durationMs,
					"error", err,
				)
			} else {
				logger.InfoContext(ctx, "request complete",
					"code", "ok",
					"duration_ms", durationMs,
				)
			}

			return resp, err
		}
	}
}
