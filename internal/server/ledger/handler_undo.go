package ledger

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/templates"
	"github.com/brendanv/float/internal/txlock"
)

// SnapshotTrailer is the response trailer carrying the snapshot an undoable
// mutation created; UndoSnapshot takes it to undo that mutation.
const SnapshotTrailer = "Float-Snapshot"

// undoableProcedures are the mutations that report their snapshot in
// SnapshotTrailer. Keep the list in UndoSnapshotRequest's proto comment in
// sync. A call only gets the trailer when it made exactly one write and every
// file it touched is covered by snapshots (see txlock.Receipt), so e.g. a
// write that also changes config.toml is never offered for undo.
var undoableProcedures = map[string]bool{
	floatv1connect.LedgerServiceDeleteTransactionProcedure:            true,
	floatv1connect.LedgerServiceUpdateTransactionProcedure:            true,
	floatv1connect.LedgerServiceBulkEditTransactionsProcedure:         true,
	floatv1connect.LedgerServiceBulkDeleteTransactionsProcedure:       true,
	floatv1connect.LedgerServiceApplyRulesProcedure:                   true,
	floatv1connect.LedgerServiceImportTransactionsProcedure:           true,
	floatv1connect.LedgerServiceImportStructuredTransactionsProcedure: true,
	floatv1connect.LedgerServiceDeleteRuleProcedure:                   true,
	floatv1connect.LedgerServiceDeleteTemplateProcedure:               true,
	floatv1connect.LedgerServiceDeletePriceProcedure:                  true,
	floatv1connect.LedgerServiceDeleteAccountDeclarationProcedure:     true,
	floatv1connect.LedgerServiceSetAccountClosedProcedure:             true,
	floatv1connect.LedgerServiceRestoreSnapshotProcedure:              true,
}

type undoInterceptor struct{}

// NewUndoInterceptor returns a server interceptor that attaches a
// txlock.Receipt to undoable procedures and, when the call succeeds with an
// undoable write, sets SnapshotTrailer. Trailers (not headers) work for
// server-streaming RPCs too, since the snapshot only exists once the stream's
// write has committed.
func NewUndoInterceptor() connect.Interceptor { return undoInterceptor{} }

func (undoInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Spec().IsClient || !undoableProcedures[req.Spec().Procedure] {
			return next(ctx, req)
		}
		ctx, receipt := txlock.WithReceipt(ctx)
		resp, err := next(ctx, req)
		if err == nil && resp != nil {
			if snapshot, ok := receipt.Undoable(); ok {
				resp.Trailer().Set(SnapshotTrailer, snapshot)
			}
		}
		return resp, err
	}
}

func (undoInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (undoInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !undoableProcedures[conn.Spec().Procedure] {
			return next(ctx, conn)
		}
		ctx, receipt := txlock.WithReceipt(ctx)
		err := next(ctx, conn)
		if err == nil {
			if snapshot, ok := receipt.Undoable(); ok {
				conn.ResponseTrailer().Set(SnapshotTrailer, snapshot)
			}
		}
		return err
	}
}

var errUndoStale = errors.New("newer changes have been saved since; restore an earlier snapshot from Snapshots instead")

// UndoSnapshot reverts the newest snapshot by restoring its parent's files as
// a new snapshot (so the undo can itself be undone, i.e. redone). It runs
// under txlock like RestoreSnapshot: serialized, validated by hledger check,
// and reverted file-for-file on failure. If the snapshot is no longer the
// newest, nothing changes and the call fails with FailedPrecondition.
func (h *Handler) UndoSnapshot(ctx context.Context, req *connect.Request[floatv1.UndoSnapshotRequest]) (*connect.Response[floatv1.UndoSnapshotResponse], error) {
	if h.snap == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("snapshots not enabled"))
	}
	hash := req.Msg.Hash
	if hash == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hash is required"))
	}
	logger := slogctx.FromContext(ctx)
	plan, undone, err := h.snap.PlanUndo(ctx, hash)
	switch {
	case errors.Is(err, gitsnap.ErrHeadMoved):
		logger.WarnContext(ctx, "undo rejected: not the newest snapshot", "hash", hash)
		return nil, connect.NewError(connect.CodeFailedPrecondition, errUndoStale)
	case errors.Is(err, gitsnap.ErrNotUndoable):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case err != nil:
		logger.WarnContext(ctx, "undo rejected", "hash", hash, "error", err)
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	msg := undoMessage(undone)
	extraPaths := append([]string{rules.FilePath(h.dataDir), templates.FilePath(h.dataDir)}, plan.Paths...)
	ctx, receipt := txlock.WithReceipt(ctx)
	err = h.lock.DoWith(ctx, msg, extraPaths, func() error {
		if err := h.snap.ApplyRestore(ctx, plan); err != nil {
			switch {
			case errors.Is(err, gitsnap.ErrHeadMoved):
				return connect.NewError(connect.CodeFailedPrecondition, errUndoStale)
			case errors.Is(err, gitsnap.ErrDirty):
				// Unsnapshotted changes are on disk (a previous snapshot commit
				// failed). Record them so they aren't lost; they are newer
				// than hash, so the undo no longer applies.
				if rerr := h.snap.RecoverUncommitted(ctx); rerr != nil {
					return rerr
				}
				return connect.NewError(connect.CodeFailedPrecondition, errUndoStale)
			}
			return err
		}
		if len(plan.Paths) == 0 {
			return txlock.ErrNoChanges
		}
		return nil
	})
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		logger.WarnContext(ctx, "undo rejected", "hash", hash, "error", err)
		return nil, cerr
	}
	if err != nil {
		return nil, rpcErr(ctx, err, "undo failed", "hash", hash)
	}
	snapshot, _ := receipt.Undoable()
	return connect.NewResponse(&floatv1.UndoSnapshotResponse{Snapshot: snapshot, Message: msg}), nil
}

// undoMessage names the snapshot that undoes one with message m, so undoing
// an undo reads as a redo rather than "undo: undo: …".
func undoMessage(m string) string {
	m = strings.TrimSpace(m)
	if rest, ok := strings.CutPrefix(m, "undo: "); ok {
		return "redo: " + rest
	}
	if rest, ok := strings.CutPrefix(m, "redo: "); ok {
		return "undo: " + rest
	}
	return "undo: " + m
}
