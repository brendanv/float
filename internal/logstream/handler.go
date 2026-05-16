package logstream

import (
	"context"
	"log/slog"
)

// BroadcastHandler wraps an existing slog.Handler and also publishes each
// handled record to a Broadcaster. The inner handler's behaviour (e.g. writing
// JSON to stderr) is unchanged.
type BroadcastHandler struct {
	inner       slog.Handler
	broadcaster *Broadcaster
	preAttrs    []slog.Attr // attrs accumulated via WithAttrs
}

// NewBroadcastHandler returns a BroadcastHandler that delegates all logging to
// inner and additionally fans records out via b.
func NewBroadcastHandler(inner slog.Handler, b *Broadcaster) *BroadcastHandler {
	return &BroadcastHandler{inner: inner, broadcaster: b}
}

func (h *BroadcastHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *BroadcastHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}
	if h.broadcaster.HasSubscribers() {
		attrs := make(map[string]string)
		for _, a := range h.preAttrs {
			flattenAttr(attrs, "", a)
		}
		r.Attrs(func(a slog.Attr) bool {
			flattenAttr(attrs, "", a)
			return true
		})
		h.broadcaster.Publish(Entry{
			Time:    r.Time,
			Level:   r.Level,
			Message: r.Message,
			Attrs:   attrs,
		})
	}
	return nil
}

func (h *BroadcastHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.preAttrs)+len(attrs))
	copy(newAttrs, h.preAttrs)
	copy(newAttrs[len(h.preAttrs):], attrs)
	return &BroadcastHandler{
		inner:       h.inner.WithAttrs(attrs),
		broadcaster: h.broadcaster,
		preAttrs:    newAttrs,
	}
}

func (h *BroadcastHandler) WithGroup(name string) slog.Handler {
	return &BroadcastHandler{
		inner:       h.inner.WithGroup(name),
		broadcaster: h.broadcaster,
		preAttrs:    h.preAttrs,
	}
}

// flattenAttr recursively converts a slog.Attr into map entries.
// Group attrs are expanded using dot-separated key paths.
func flattenAttr(m map[string]string, prefix string, a slog.Attr) {
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			flattenAttr(m, key, ga)
		}
	} else {
		m[key] = a.Value.String()
	}
}
