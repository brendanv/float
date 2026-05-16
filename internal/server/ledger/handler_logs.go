package ledger

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
)

// StreamLogs streams server log entries to the client until disconnection.
// An optional min_level filter (DEBUG/INFO/WARN/ERROR) limits which entries
// are sent; entries below the threshold are silently skipped. Entries are
// dropped (not buffered indefinitely) when the client cannot keep up.
func (h *Handler) StreamLogs(
	ctx context.Context,
	req *connect.Request[floatv1.StreamLogsRequest],
	stream *connect.ServerStream[floatv1.StreamLogsResponse],
) error {
	if h.logBroadcaster == nil {
		<-ctx.Done()
		return nil
	}

	minLevel := slog.LevelInfo
	if req.Msg.MinLevel != "" {
		if err := minLevel.UnmarshalText([]byte(req.Msg.MinLevel)); err != nil {
			return connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	ch, unsub := h.logBroadcaster.Subscribe(256)
	defer unsub()

	for {
		select {
		case <-ctx.Done():
			return nil
		case entry := <-ch:
			if entry.Level < minLevel {
				continue
			}
			if err := stream.Send(&floatv1.StreamLogsResponse{
				Entry: &floatv1.LogEntry{
					Time:    entry.Time.Format(time.RFC3339Nano),
					Level:   entry.Level.String(),
					Message: entry.Message,
					Attrs:   entry.Attrs,
				},
			}); err != nil {
				return err
			}
		}
	}
}
