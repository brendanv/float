package slogctx

import (
	"context"
	"log/slog"
)

type key struct{}

// WithLogger returns a copy of ctx with the given logger stored in it.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, key{}, l)
}

// FromContext retrieves the logger stored in ctx. If none is present, it
// returns slog.Default().
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(key{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
