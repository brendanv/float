package ledger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/templates"
	"github.com/brendanv/float/internal/txlock"
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

// errRestoreRetry signals that a restore's plan went stale before it could be
// applied (another write committed first) and should be re-planned.
var errRestoreRetry = errors.New("restore plan is stale")

const restoreAttempts = 3

func (h *Handler) RestoreSnapshot(ctx context.Context, req *connect.Request[floatv1.RestoreSnapshotRequest]) (*connect.Response[floatv1.RestoreSnapshotResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	hash := req.Msg.Hash
	if hash == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hash is required"))
	}
	for attempt := 1; ; attempt++ {
		err := h.restoreSnapshotOnce(ctx, hash, req.Msg.HardReset)
		if errors.Is(err, errRestoreRetry) && attempt < restoreAttempts {
			continue
		}
		if errors.Is(err, errRestoreRetry) {
			err = connect.NewError(connect.CodeAborted, errors.New("other writes kept landing during the restore; try again"))
		}
		var cerr *connect.Error
		if errors.As(err, &cerr) {
			slogctx.FromContext(ctx).WarnContext(ctx, "restore snapshot rejected", "hash", hash, "error", err)
			return nil, cerr
		}
		if err != nil {
			return nil, rpcErr(ctx, err, "restore snapshot failed", "hash", hash, "hard_reset", req.Msg.HardReset)
		}
		return connect.NewResponse(&floatv1.RestoreSnapshotResponse{}), nil
	}
}

// restoreSnapshotOnce sets the data directory back to hash's files under
// txlock, so the restore is serialized against other writes, validated with
// hledger check, and reverted file-for-file if invalid (the plan's paths are
// declared to txlock). The forward restore is then committed by txlock as a
// new snapshot. A hard reset applies the same files, and only once the check
// has passed moves the branch back to hash (so txlock's commit is a no-op);
// a failed check therefore leaves history untouched.
func (h *Handler) restoreSnapshotOnce(ctx context.Context, hash string, hardReset bool) error {
	plan, err := h.snap.PlanRestore(ctx, hash)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	extraPaths := append([]string{rules.FilePath(h.dataDir), templates.FilePath(h.dataDir)}, plan.Paths...)
	short := plan.Target[:8]
	msg := fmt.Sprintf("restore snapshot %s (%q)", short, strings.TrimSpace(plan.Message))
	if hardReset {
		msg = "hard reset to snapshot " + short
	}
	var resetErr error
	err = h.lock.DoWithPublish(ctx, msg, extraPaths, func() (func(), error) {
		if err := h.snap.ApplyRestore(ctx, plan); err != nil {
			switch {
			case errors.Is(err, gitsnap.ErrHeadMoved):
				return nil, errRestoreRetry
			case errors.Is(err, gitsnap.ErrDirty):
				// A previous snapshot commit failed. Record those files first so
				// they stay recoverable and are not folded into this restore.
				if rerr := h.snap.RecoverUncommitted(ctx); rerr != nil {
					return nil, rerr
				}
				return nil, errRestoreRetry
			}
			return nil, err
		}
		if !hardReset {
			if len(plan.Paths) == 0 {
				return nil, txlock.ErrNoChanges
			}
			return nil, nil
		}
		return func() {
			resetErr = h.snap.ResetHard(ctx, plan.Target)
		}, nil
	})
	if err != nil {
		return err
	}
	if resetErr != nil {
		// The files were restored and txlock committed them as a new snapshot;
		// only discarding the later history failed.
		return fmt.Errorf("files restored as a new snapshot, but discarding later history failed: %w", resetErr)
	}
	return nil
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
