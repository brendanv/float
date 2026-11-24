package ledger

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/templates"
)

func (h *Handler) ListSnapshots(ctx context.Context, req *connect.Request[floatv1.ListSnapshotsRequest]) (*connect.Response[floatv1.ListSnapshotsResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	snaps, err := h.snap.List(ctx, int(req.Msg.Limit))
	if err != nil {
		return nil, rpcErr(ctx, err, "list snapshots failed")
	}
	out := make([]*floatv1.Snapshot, len(snaps))
	for i, s := range snaps {
		out[i] = &floatv1.Snapshot{
			Hash:      s.Hash,
			Message:   s.Message,
			Timestamp: s.Timestamp.Format(time.RFC3339),
		}
	}
	return connect.NewResponse(&floatv1.ListSnapshotsResponse{Snapshots: out}), nil
}

func (h *Handler) RestoreSnapshot(ctx context.Context, req *connect.Request[floatv1.RestoreSnapshotRequest]) (*connect.Response[floatv1.RestoreSnapshotResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	if req.Msg.Hash == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hash is required"))
	}
	// Run the restore under txlock: it rewrites the whole data directory, so it
	// must be serialized against concurrent mutations, validated with hledger
	// check, and rolled back if the restored tree is invalid. DoWith also bumps
	// the generation, invalidating the query cache.
	extraPaths := []string{rules.FilePath(h.dataDir), templates.FilePath(h.dataDir)}
	if err := h.lock.DoWith(ctx, "restore snapshot "+req.Msg.Hash, extraPaths, func() error {
		return h.snap.Restore(ctx, req.Msg.Hash)
	}); err != nil {
		return nil, rpcErr(ctx, err, "restore snapshot failed", "hash", req.Msg.Hash)
	}
	return connect.NewResponse(&floatv1.RestoreSnapshotResponse{}), nil
}

func (h *Handler) GetSnapshotDiff(ctx context.Context, req *connect.Request[floatv1.GetSnapshotDiffRequest]) (*connect.Response[floatv1.GetSnapshotDiffResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	if req.Msg.Hash == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hash is required"))
	}
	files, err := h.snap.Diff(ctx, req.Msg.Hash)
	if err != nil {
		return nil, rpcErr(ctx, err, "snapshot diff failed", "hash", req.Msg.Hash)
	}
	out := make([]*floatv1.FileDiff, len(files))
	for i, f := range files {
		out[i] = &floatv1.FileDiff{
			Path:       f.Path,
			OldPath:    f.OldPath,
			ChangeType: changeTypeString(f.Change),
			IsBinary:   f.IsBinary,
			Patch:      f.Patch,
		}
	}
	return connect.NewResponse(&floatv1.GetSnapshotDiffResponse{Hash: req.Msg.Hash, Files: out}), nil
}

func changeTypeString(c gitsnap.ChangeType) string {
	switch c {
	case gitsnap.ChangeAdded:
		return "added"
	case gitsnap.ChangeDeleted:
		return "deleted"
	case gitsnap.ChangeRenamed:
		return "renamed"
	default:
		return "modified"
	}
}
